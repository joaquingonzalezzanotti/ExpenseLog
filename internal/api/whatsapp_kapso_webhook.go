package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	kapsoWebhookEventHeader       = "X-Webhook-Event"
	kapsoWebhookSignatureHeader   = "X-Webhook-Signature"
	kapsoWebhookIdempotencyHeader = "X-Idempotency-Key"

	kapsoAPIBaseURL              = "https://api.kapso.ai/meta/whatsapp/v24.0"
	kapsoWebhookMaxBodyBytes     = 1 << 20
	kapsoWebhookProcessedKeyTTL  = 30 * time.Minute
	kapsoWebhookOutboundTimeout  = 8 * time.Second
	kapsoWebhookFailureBodyLimit = 4096
	kapsoWebhookMediaMaxBytes    = 8 * 1024 * 1024
	aiParserDefaultParsePath     = "/api/parse"
)

var (
	whatsAppExpenseCommandRE = regexp.MustCompile(`(?i)^/?(gasto|ingreso|reintegro|refund)\s+([0-9]+(?:[.,][0-9]{1,2})?)\s*(.*)$`)
	whatsAppLinkCommandRE    = regexp.MustCompile(`(?i)^/?vincular\s+([a-z0-9\-\s]+)$`)
	whatsAppAmountTokenRE    = regexp.MustCompile(`\$?\s*([0-9]+(?:[.,][0-9]{3})*(?:[.,][0-9]{1,2})?)`)
	whatsAppDescHintRE       = regexp.MustCompile(`(?i)\b(?:en|a|de)\s+(.+)$`)
	whatsAppURLRE            = regexp.MustCompile(`https?://\S+`)
)

var kapsoWebhookProcessedKeys = struct {
	mu   sync.Mutex
	keys map[string]time.Time
}{
	keys: make(map[string]time.Time),
}

type kapsoWebhookEnvelope struct {
	Event  string            `json:"event"`
	Type   string            `json:"type"`
	Data   json.RawMessage   `json:"data"`
	Events []json.RawMessage `json:"events"`
}

type kapsoMessageEvent struct {
	Message       kapsoWebhookMessage      `json:"message"`
	Conversation  kapsoWebhookConversation `json:"conversation"`
	PhoneNumberID string                   `json:"phone_number_id"`
}

type kapsoWebhookMessage struct {
	ID       string                   `json:"id"`
	From     string                   `json:"from"`
	Type     string                   `json:"type"`
	Text     kapsoWebhookMessageText  `json:"text"`
	Image    kapsoWebhookMessageMedia `json:"image"`
	Video    kapsoWebhookMessageMedia `json:"video"`
	Document kapsoWebhookMessageMedia `json:"document"`
	Kapso    kapsoWebhookMessageKapso `json:"kapso"`
}

type kapsoWebhookMessageText struct {
	Body string `json:"body"`
}

type kapsoWebhookMessageKapso struct {
	Direction       string                       `json:"direction"`
	Content         string                       `json:"content"`
	HasMedia        bool                         `json:"has_media"`
	MediaURL        string                       `json:"media_url"`
	MediaData       kapsoWebhookMessageMediaData `json:"media_data"`
	MessageTypeData kapsoWebhookMessageTypeData  `json:"message_type_data"`
}

type kapsoWebhookMessageMedia struct {
	ID       string `json:"id"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
}

type kapsoWebhookMessageTypeData struct {
	Caption string `json:"caption"`
}

type kapsoWebhookMessageMediaData struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
}

type kapsoWebhookConversation struct {
	PhoneNumber   string `json:"phone_number"`
	PhoneNumberID string `json:"phone_number_id"`
}

type kapsoSendMessagePayload struct {
	MessagingProduct string                      `json:"messaging_product"`
	RecipientType    string                      `json:"recipient_type"`
	To               string                      `json:"to"`
	Type             string                      `json:"type"`
	Text             kapsoSendMessagePayloadText `json:"text"`
}

type kapsoSendMessagePayloadText struct {
	Body string `json:"body"`
}

type whatsAppParsedExpense struct {
	Flow     string
	Amount   float64
	Name     string
	Category string
	Currency string
	Source   string
	Date     time.Time
}

type whatsAppAIParserRequest struct {
	Text        string                 `json:"text,omitempty"`
	ContextDate string                 `json:"context_date,omitempty"`
	Media       *whatsAppAIParserMedia `json:"media,omitempty"`
}

type whatsAppAIParserMedia struct {
	DataBase64 string `json:"data_base64"`
	MimeType   string `json:"mime_type"`
	Filename   string `json:"filename,omitempty"`
}

type whatsAppAIParserResponse struct {
	Type            string   `json:"type"`
	Amount          float64  `json:"amount"`
	Currency        string   `json:"currency"`
	DateTimeISO     string   `json:"datetime_iso"`
	Counterparty    string   `json:"counterparty"`
	Reference       string   `json:"reference"`
	Motive          string   `json:"motive"`
	SourceApp       string   `json:"source_app"`
	MissingRequired []string `json:"missing_required"`
}

type kapsoMediaContext struct {
	URL      string
	MimeType string
	Filename string
	TextHint string
}

func (h *Handler) HandleKapsoWhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}

	bodyReader := http.MaxBytesReader(w, r.Body, kapsoWebhookMaxBodyBytes)
	defer bodyReader.Close()
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid webhook body"})
		return
	}

	signature := strings.TrimSpace(r.Header.Get(kapsoWebhookSignatureHeader))
	secret := strings.TrimSpace(os.Getenv("KAPSO_WEBHOOK_SECRET"))
	if secret != "" && !isKapsoWebhookSignatureValid(body, signature, secret) {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Invalid webhook signature"})
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(kapsoWebhookIdempotencyHeader))
	if idempotencyKey != "" {
		if kapsoWebhookAlreadyProcessed(idempotencyKey, time.Now()) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
	}

	eventHeader := strings.TrimSpace(r.Header.Get(kapsoWebhookEventHeader))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))

	go h.processKapsoWhatsAppWebhook(body, eventHeader)
}

func (h *Handler) processKapsoWhatsAppWebhook(body []byte, eventHeader string) {
	eventType := strings.TrimSpace(eventHeader)
	payloads := make([]json.RawMessage, 0, 1)

	var envelope kapsoWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil {
		if eventType == "" {
			eventType = strings.TrimSpace(envelope.Event)
		}
		if eventType == "" {
			eventType = strings.TrimSpace(envelope.Type)
		}
		if len(envelope.Events) > 0 {
			payloads = append(payloads, envelope.Events...)
		}
		if len(envelope.Data) > 0 {
			payloads = append(payloads, envelope.Data)
		}
	}
	if len(payloads) == 0 {
		payloads = append(payloads, json.RawMessage(append([]byte(nil), body...)))
	}

	for _, raw := range payloads {
		var event kapsoMessageEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			log.Printf("WHATSAPP KAPSO: ignoring payload (decode failed): %v", err)
			continue
		}
		if !isKapsoInboundMessageEvent(eventType, event) {
			continue
		}
		h.handleKapsoInboundMessage(event)
	}
}

func (h *Handler) handleKapsoInboundMessage(event kapsoMessageEvent) {
	phoneNumberID := strings.TrimSpace(event.PhoneNumberID)
	if phoneNumberID == "" {
		phoneNumberID = strings.TrimSpace(event.Conversation.PhoneNumberID)
	}
	if phoneNumberID == "" {
		log.Printf("WHATSAPP KAPSO: ignoring inbound message without phone_number_id")
		return
	}

	fromPhone := normalizeDialNumber(event.Conversation.PhoneNumber)
	if fromPhone == "" {
		fromPhone = normalizeDialNumber(event.Message.From)
	}
	if fromPhone == "" {
		log.Printf("WHATSAPP KAPSO: ignoring inbound message without sender phone")
		return
	}

	textBody := strings.TrimSpace(event.Message.Text.Body)
	if textBody == "" {
		textBody = strings.TrimSpace(event.Message.Kapso.Content)
	}

	if compactCode := extractWhatsAppLinkCodeFromText(textBody); compactCode != "" {
		reply := h.consumeWhatsAppLinkCode(compactCode, fromPhone)
		if err := sendKapsoTextMessage(phoneNumberID, fromPhone, reply); err != nil {
			log.Printf("WHATSAPP KAPSO: failed sending link reply to %s: %v", fromPhone, err)
		}
		return
	}

	link, err := h.storage.GetWhatsAppUserLinkByPhone(fromPhone)
	if err == sql.ErrNoRows {
		reply := "Tu numero no esta vinculado a ExpenseLog. Entra a Ajustes > WhatsApp Bot, genera un codigo y envia: /vincular CODIGO"
		if sendErr := sendKapsoTextMessage(phoneNumberID, fromPhone, reply); sendErr != nil {
			log.Printf("WHATSAPP KAPSO: failed sending unlinked reply to %s: %v", fromPhone, sendErr)
		}
		return
	}
	if err != nil {
		log.Printf("WHATSAPP KAPSO: failed resolving link by phone %s: %v", fromPhone, err)
		return
	}

	premium, err := h.isUserPremium(link.UserID)
	if err != nil {
		log.Printf("WHATSAPP KAPSO: failed checking premium for user %s: %v", link.UserID, err)
		return
	}
	if !premium {
		reply := "Tu cuenta ExpenseLog ya no tiene Premium activo. Activalo para usar el bot de WhatsApp."
		if sendErr := sendKapsoTextMessage(phoneNumberID, fromPhone, reply); sendErr != nil {
			log.Printf("WHATSAPP KAPSO: failed sending premium reply to %s: %v", fromPhone, sendErr)
		}
		return
	}

	if event.Message.Kapso.HasMedia || (strings.TrimSpace(event.Message.Type) != "" && !strings.EqualFold(strings.TrimSpace(event.Message.Type), "text")) {
		reply := h.createExpenseFromWhatsAppMedia(link.UserID, event)
		if sendErr := sendKapsoTextMessage(phoneNumberID, fromPhone, reply); sendErr != nil {
			log.Printf("WHATSAPP KAPSO: failed sending media reply to %s: %v", fromPhone, sendErr)
		}
		return
	}

	reply := h.createExpenseFromWhatsAppText(link.UserID, textBody)
	if sendErr := sendKapsoTextMessage(phoneNumberID, fromPhone, reply); sendErr != nil {
		log.Printf("WHATSAPP KAPSO: failed sending command reply to %s: %v", fromPhone, sendErr)
	}
}

func (h *Handler) consumeWhatsAppLinkCode(compactCode, whatsAppPhone string) string {
	_, err := h.storage.ConsumeWhatsAppLinkCode(
		hashWhatsAppLinkCode(compactCode),
		whatsAppPhone,
		time.Now().UTC(),
	)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrWhatsAppInvalidLinkCode):
			return "El codigo no es valido."
		case errors.Is(err, storage.ErrWhatsAppLinkCodeExpired):
			return "El codigo ya vencio. Genera uno nuevo en ExpenseLog."
		case errors.Is(err, storage.ErrWhatsAppLinkCodeUsed):
			return "El codigo ya fue usado."
		case errors.Is(err, storage.ErrWhatsAppPremiumRequired):
			return "Necesitas Premium activo para vincular WhatsApp."
		case errors.Is(err, storage.ErrWhatsAppAlreadyLinked):
			return "Tu cuenta de ExpenseLog ya estaba vinculada a otro numero."
		case errors.Is(err, storage.ErrWhatsAppPhoneAlreadyLinked):
			return "Este numero ya esta vinculado a otra cuenta de ExpenseLog."
		default:
			log.Printf("WHATSAPP KAPSO: consume link code failed: %v", err)
			return "No pude completar la vinculacion. Reintenta en unos segundos."
		}
	}
	return "Cuenta vinculada correctamente. Ya puedes registrar gastos con /gasto 1500 Supermercado"
}

func extractWhatsAppLinkCodeFromText(text string) string {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return ""
	}
	matches := whatsAppLinkCommandRE.FindStringSubmatch(raw)
	if len(matches) < 2 {
		return ""
	}
	return normalizeWhatsAppLinkCode(matches[1])
}

func parseWhatsAppExpenseText(rawText, defaultCurrency string) (whatsAppParsedExpense, error) {
	text := strings.TrimSpace(rawText)
	if text == "" {
		return whatsAppParsedExpense{}, fmt.Errorf("mensaje vacio")
	}
	if strings.EqualFold(text, "/ayuda") || strings.EqualFold(text, "ayuda") || strings.EqualFold(text, "/help") || strings.EqualFold(text, "help") {
		return whatsAppParsedExpense{}, fmt.Errorf("help")
	}
	if parsed, ok, err := parseWhatsAppExplicitCommand(text, defaultCurrency); ok || err != nil {
		return parsed, err
	}
	return parseWhatsAppNaturalText(text, defaultCurrency)
}

func (h *Handler) createExpenseFromWhatsAppText(userID, text string) string {
	defaultCurrency, err := h.storage.GetCurrency(userID)
	if err != nil || strings.TrimSpace(defaultCurrency) == "" {
		defaultCurrency = "ars"
	}

	parsed, parseErr := parseWhatsAppExpenseText(text, defaultCurrency)
	if parseErr != nil {
		if parseErr.Error() == "help" {
			return "Comandos: /gasto 1500 Supermercado | /ingreso 50000 Sueldo | /reintegro 1200 Devolucion | o texto: gaste 2000 en super"
		}
		aiParsed, aiErr := h.tryParseWhatsAppWithAI(text, nil, defaultCurrency)
		if aiErr != nil {
			return "No pude interpretar el mensaje. Usa: /gasto 1500 Supermercado o texto: gaste 2000 en super"
		}
		parsed = aiParsed
	}

	return h.saveWhatsAppParsedExpense(userID, parsed)
}

func (h *Handler) createExpenseFromWhatsAppMedia(userID string, event kapsoMessageEvent) string {
	defaultCurrency, err := h.storage.GetCurrency(userID)
	if err != nil || strings.TrimSpace(defaultCurrency) == "" {
		defaultCurrency = "ars"
	}

	media := extractKapsoMediaContext(event)
	if media.URL == "" {
		return "Recibi tu archivo, pero no encontre el media_url en Kapso. Reenvialo o usa texto: gaste 1500 en super."
	}

	mediaBytes, detectedMimeType, err := downloadKapsoMedia(media.URL)
	if err != nil {
		log.Printf("WHATSAPP KAPSO: failed downloading media: %v", err)
		return "Recibi tu archivo, pero no pude descargarlo para procesarlo. Reintenta en unos segundos."
	}
	if media.MimeType == "" {
		media.MimeType = detectedMimeType
	}
	if media.MimeType == "" {
		media.MimeType = "application/octet-stream"
	}

	aiParsed, aiErr := h.tryParseWhatsAppWithAI(media.TextHint, &whatsAppAIParserMedia{
		DataBase64: base64.StdEncoding.EncodeToString(mediaBytes),
		MimeType:   media.MimeType,
		Filename:   media.Filename,
	}, defaultCurrency)
	if aiErr != nil {
		log.Printf("WHATSAPP KAPSO: ai parse for media failed: %v", aiErr)
		return "Recibi tu archivo, pero no pude extraer el gasto automaticamente. Proba con texto: gaste 1500 en super."
	}

	return h.saveWhatsAppParsedExpense(userID, aiParsed)
}

func (h *Handler) saveWhatsAppParsedExpense(userID string, parsed whatsAppParsedExpense) string {
	flow, adjustedAmount, err := normalizeFlow(parsed.Flow, parsed.Amount)
	if err != nil {
		return "No pude interpretar el tipo de movimiento. Usa /gasto, /ingreso o /reintegro."
	}

	source := strings.TrimSpace(parsed.Source)
	if source == "" {
		source = "CA"
	}
	when := parsed.Date
	if when.IsZero() {
		when = time.Now().UTC()
	}

	expense := storage.Expense{
		ID:           uuid.New().String(),
		Flow:         flow,
		Name:         parsed.Name,
		Category:     parsed.Category,
		Amount:       adjustedAmount,
		Currency:     parsed.Currency,
		Source:       source,
		Tags:         uniqueTags([]string{"whatsapp_bot"}),
		SystemOrigin: "whatsapp_bot",
		Date:         when,
	}
	if err := expense.Validate(); err != nil {
		return "No pude crear el movimiento. Verifica formato y reintenta."
	}
	if err := h.storage.AddExpense(userID, expense); err != nil {
		log.Printf("WHATSAPP KAPSO: add expense failed for user %s: %v", userID, err)
		return "No pude guardar el movimiento en ExpenseLog. Reintenta en unos segundos."
	}
	return fmt.Sprintf("Movimiento registrado: %s %.2f (%s).", strings.ToUpper(flow), math.Abs(adjustedAmount), strings.ToUpper(expense.Currency))
}

func parseWhatsAppExplicitCommand(text, defaultCurrency string) (whatsAppParsedExpense, bool, error) {
	matches := whatsAppExpenseCommandRE.FindStringSubmatch(text)
	if len(matches) < 4 {
		return whatsAppParsedExpense{}, false, nil
	}

	flow, err := mapWhatsAppFlowToken(matches[1])
	if err != nil {
		return whatsAppParsedExpense{}, true, err
	}
	amount, err := parseWhatsAppAmountToken(matches[2])
	if err != nil {
		return whatsAppParsedExpense{}, true, fmt.Errorf("invalid_amount")
	}

	description := storage.SanitizeString(strings.TrimSpace(matches[3]))
	parsed := buildParsedWhatsAppExpense(flow, amount, description, defaultCurrency)
	return parsed, true, nil
}

func parseWhatsAppNaturalText(text, defaultCurrency string) (whatsAppParsedExpense, error) {
	flow := inferWhatsAppFlowFromText(text)
	if flow == "" {
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_format")
	}

	amountToken := extractWhatsAppAmountToken(text)
	if amountToken == "" {
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_amount")
	}
	amount, err := parseWhatsAppAmountToken(amountToken)
	if err != nil {
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_amount")
	}

	description := inferWhatsAppDescription(text, amountToken)
	return buildParsedWhatsAppExpense(flow, amount, description, defaultCurrency), nil
}

func inferWhatsAppFlowFromText(text string) string {
	norm := normalizeWhatsAppNaturalText(text)
	switch {
	case strings.Contains(norm, "reintegro"),
		strings.Contains(norm, "devolucion"),
		strings.Contains(norm, "devolvieron"),
		strings.Contains(norm, "refund"),
		strings.Contains(norm, "cashback"):
		return "refund"
	case strings.Contains(norm, "ingreso"),
		strings.Contains(norm, "ingrese"),
		strings.Contains(norm, "cobre"),
		strings.Contains(norm, "cobro"),
		strings.Contains(norm, "recibi"),
		strings.Contains(norm, "recibieron"),
		strings.Contains(norm, "depositaron"),
		strings.Contains(norm, "me transfirieron"):
		return "income"
	case strings.Contains(norm, "gasto"),
		strings.Contains(norm, "gaste"),
		strings.Contains(norm, "pago"),
		strings.Contains(norm, "pague"),
		strings.Contains(norm, "compre"),
		strings.Contains(norm, "compra"),
		strings.Contains(norm, "abone"),
		strings.Contains(norm, "abono"):
		return "expense"
	default:
		return ""
	}
}

func mapWhatsAppFlowToken(flowToken string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(flowToken)) {
	case "gasto", "expense", "egreso":
		return "expense", nil
	case "ingreso", "income":
		return "income", nil
	case "reintegro", "refund", "devolucion":
		return "refund", nil
	default:
		return "", fmt.Errorf("invalid_flow")
	}
}

func buildParsedWhatsAppExpense(flow string, amount float64, description, defaultCurrency string) whatsAppParsedExpense {
	category := "Varios"
	if flow == "income" || flow == "refund" {
		category = "Ingresos"
	}

	name := storage.SanitizeString(strings.TrimSpace(description))
	if name == "" {
		if flow == "expense" {
			name = "Movimiento WhatsApp"
		} else {
			name = "Ingreso WhatsApp"
		}
	}

	currency := strings.ToLower(strings.TrimSpace(defaultCurrency))
	if currency == "" {
		currency = "ars"
	}

	return whatsAppParsedExpense{
		Flow:     flow,
		Amount:   math.Abs(amount),
		Name:     name,
		Category: category,
		Currency: currency,
	}
}

func normalizeWhatsAppNaturalText(text string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ä", "a", "â", "a",
		"é", "e", "è", "e", "ë", "e", "ê", "e",
		"í", "i", "ì", "i", "ï", "i", "î", "i",
		"ó", "o", "ò", "o", "ö", "o", "ô", "o",
		"ú", "u", "ù", "u", "ü", "u", "û", "u",
		"Á", "A", "À", "A", "Ä", "A", "Â", "A",
		"É", "E", "È", "E", "Ë", "E", "Ê", "E",
		"Í", "I", "Ì", "I", "Ï", "I", "Î", "I",
		"Ó", "O", "Ò", "O", "Ö", "O", "Ô", "O",
		"Ú", "U", "Ù", "U", "Ü", "U", "Û", "U",
	)
	return strings.ToLower(strings.TrimSpace(replacer.Replace(text)))
}

func extractWhatsAppAmountToken(text string) string {
	matches := whatsAppAmountTokenRE.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parseWhatsAppAmountToken(raw string) (float64, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimLeft(clean, "$")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, fmt.Errorf("empty amount")
	}

	hasComma := strings.Contains(clean, ",")
	hasDot := strings.Contains(clean, ".")

	switch {
	case hasComma && hasDot:
		lastComma := strings.LastIndex(clean, ",")
		lastDot := strings.LastIndex(clean, ".")
		if lastComma > lastDot {
			clean = strings.ReplaceAll(clean, ".", "")
			clean = strings.ReplaceAll(clean, ",", ".")
		} else {
			clean = strings.ReplaceAll(clean, ",", "")
		}
	case hasComma:
		if strings.Count(clean, ",") > 1 {
			clean = strings.ReplaceAll(clean, ",", "")
		} else {
			parts := strings.Split(clean, ",")
			if len(parts) == 2 && len(parts[1]) == 3 {
				clean = strings.ReplaceAll(clean, ",", "")
			} else {
				clean = strings.ReplaceAll(clean, ",", ".")
			}
		}
	case hasDot:
		if strings.Count(clean, ".") > 1 {
			clean = strings.ReplaceAll(clean, ".", "")
		} else {
			parts := strings.Split(clean, ".")
			if len(parts) == 2 && len(parts[1]) == 3 {
				clean = strings.ReplaceAll(clean, ".", "")
			}
		}
	}

	amount, err := strconvParseFloat(clean)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid amount")
	}
	return math.Abs(amount), nil
}

func inferWhatsAppDescription(text, amountToken string) string {
	if matches := whatsAppDescHintRE.FindStringSubmatch(text); len(matches) >= 2 {
		desc := storage.SanitizeString(strings.TrimSpace(matches[1]))
		if desc != "" {
			return desc
		}
	}

	base := strings.Replace(text, amountToken, "", 1)
	base = strings.TrimSpace(base)
	base = strings.TrimPrefix(base, "/")
	base = strings.TrimSpace(base)
	return storage.SanitizeString(base)
}

func (h *Handler) tryParseWhatsAppWithAI(text string, media *whatsAppAIParserMedia, defaultCurrency string) (whatsAppParsedExpense, error) {
	if !isWhatsAppAIFallbackEnabled() {
		return whatsAppParsedExpense{}, fmt.Errorf("ai fallback disabled")
	}

	aiResponse, err := callWhatsAppAIParser(text, media)
	if err != nil {
		return whatsAppParsedExpense{}, err
	}
	if len(aiResponse.MissingRequired) > 0 {
		return whatsAppParsedExpense{}, fmt.Errorf("ai response incomplete: %s", strings.Join(aiResponse.MissingRequired, ","))
	}

	flow, err := mapWhatsAppFlowToken(aiResponse.Type)
	if err != nil {
		return whatsAppParsedExpense{}, fmt.Errorf("ai response missing flow")
	}
	amount := math.Abs(aiResponse.Amount)
	if amount <= 0 {
		return whatsAppParsedExpense{}, fmt.Errorf("ai response invalid amount")
	}

	currency := strings.ToLower(strings.TrimSpace(aiResponse.Currency))
	if currency == "" {
		currency = strings.ToLower(strings.TrimSpace(defaultCurrency))
	}
	if currency == "" {
		currency = "ars"
	}

	counterparty := storage.SanitizeString(strings.TrimSpace(aiResponse.Counterparty))
	motive := storage.SanitizeString(strings.TrimSpace(aiResponse.Motive))
	reference := storage.SanitizeString(strings.TrimSpace(aiResponse.Reference))
	name := counterparty
	if name == "" {
		name = "Movimiento WhatsApp"
	}
	switch {
	case motive != "" && reference != "":
		name = fmt.Sprintf("%s - %s (%s)", name, motive, reference)
	case motive != "":
		name = fmt.Sprintf("%s - %s", name, motive)
	case reference != "":
		name = fmt.Sprintf("%s (%s)", name, reference)
	}

	category := "Varios"
	if flow == "income" || flow == "refund" {
		category = "Ingresos"
	}

	date := parseTelegramDateTime(aiResponse.DateTimeISO, time.Now().UTC())
	return whatsAppParsedExpense{
		Flow:     flow,
		Amount:   amount,
		Name:     name,
		Category: category,
		Currency: currency,
		Source:   normalizeBotExpenseSource(aiResponse.SourceApp),
		Date:     date,
	}, nil
}

func callWhatsAppAIParser(text string, media *whatsAppAIParserMedia) (whatsAppAIParserResponse, error) {
	endpoint := strings.TrimSpace(os.Getenv("AI_PARSER_BASE_URL"))
	if endpoint == "" {
		return whatsAppAIParserResponse{}, fmt.Errorf("AI_PARSER_BASE_URL is not configured")
	}
	endpoint = strings.TrimRight(endpoint, "/")

	parsePath := strings.TrimSpace(os.Getenv("AI_PARSER_PARSE_PATH"))
	if parsePath == "" {
		parsePath = aiParserDefaultParsePath
	}
	if !strings.HasPrefix(parsePath, "/") {
		parsePath = "/" + parsePath
	}
	endpoint = endpoint + parsePath

	payload := whatsAppAIParserRequest{
		Text:        strings.TrimSpace(text),
		ContextDate: time.Now().UTC().Format(time.RFC3339),
		Media:       media,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return whatsAppAIParserResponse{}, err
	}

	timeout := aiParserTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encodedPayload))
	if err != nil {
		return whatsAppAIParserResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	internalToken := strings.TrimSpace(os.Getenv("AI_PARSER_INTERNAL_TOKEN"))
	if internalToken != "" {
		req.Header.Set("X-Internal-Token", internalToken)
	}

	apiKey := strings.TrimSpace(os.Getenv("AI_PARSER_API_KEY"))
	if apiKey != "" {
		headerName := strings.TrimSpace(os.Getenv("AI_PARSER_API_KEY_HEADER"))
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, apiKey)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return whatsAppAIParserResponse{}, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, kapsoWebhookFailureBodyLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return whatsAppAIParserResponse{}, fmt.Errorf("ai parser failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var parsed whatsAppAIParserResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return whatsAppAIParserResponse{}, err
	}
	return parsed, nil
}

func aiParserTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AI_PARSER_TIMEOUT_MS"))
	if raw == "" {
		return 10 * time.Second
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 10 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func isWhatsAppAIFallbackEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("WHATSAPP_AI_FALLBACK_ENABLED"))
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func extractKapsoMediaContext(event kapsoMessageEvent) kapsoMediaContext {
	textHint := strings.TrimSpace(event.Message.Kapso.MessageTypeData.Caption)
	if textHint == "" {
		textHint = strings.TrimSpace(event.Message.Image.Caption)
	}
	if textHint == "" {
		textHint = strings.TrimSpace(event.Message.Document.Caption)
	}
	if textHint == "" {
		textHint = strings.TrimSpace(event.Message.Video.Caption)
	}
	if textHint == "" {
		textHint = strings.TrimSpace(event.Message.Text.Body)
	}
	if textHint == "" {
		textHint = cleanKapsoContentHint(event.Message.Kapso.Content)
	}

	mediaURL := strings.TrimSpace(event.Message.Kapso.MediaData.URL)
	if mediaURL == "" {
		mediaURL = strings.TrimSpace(event.Message.Kapso.MediaURL)
	}

	mimeType := strings.ToLower(strings.TrimSpace(event.Message.Kapso.MediaData.ContentType))
	if mimeType == "" {
		mimeType = strings.ToLower(strings.TrimSpace(event.Message.Document.MimeType))
	}

	filename := strings.TrimSpace(event.Message.Kapso.MediaData.Filename)
	if filename == "" {
		filename = strings.TrimSpace(event.Message.Document.Filename)
	}

	return kapsoMediaContext{
		URL:      mediaURL,
		MimeType: mimeType,
		Filename: filename,
		TextHint: textHint,
	}
}

func cleanKapsoContentHint(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	text = whatsAppURLRE.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)
	if len(text) > 400 {
		text = strings.TrimSpace(text[:400])
	}
	return text
}

func downloadKapsoMedia(mediaURL string) ([]byte, string, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return nil, "", fmt.Errorf("empty media url")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "*/*")
	if kapsoAPIKey := strings.TrimSpace(os.Getenv("KAPSO_API_KEY")); kapsoAPIKey != "" {
		req.Header.Set("X-API-Key", kapsoAPIKey)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, kapsoWebhookFailureBodyLimit))
		return nil, "", fmt.Errorf("kapso media download failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	limitedReader := io.LimitReader(resp.Body, kapsoWebhookMediaMaxBytes+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", err
	}
	if int64(len(bodyBytes)) > kapsoWebhookMediaMaxBytes {
		return nil, "", fmt.Errorf("media exceeds limit of %d bytes", kapsoWebhookMediaMaxBytes)
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return bodyBytes, contentType, nil
}

func isKapsoWebhookSignatureValid(body []byte, signature, secret string) bool {
	signature = strings.TrimSpace(signature)
	secret = strings.TrimSpace(secret)
	if signature == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bytes.TrimSpace(body))
	expectedHex := hex.EncodeToString(mac.Sum(nil))
	expected := []byte(expectedHex)
	received := []byte(signature)
	if len(received) != len(expected) {
		return false
	}
	return hmac.Equal(received, expected)
}

func kapsoWebhookAlreadyProcessed(key string, now time.Time) bool {
	kapsoWebhookProcessedKeys.mu.Lock()
	defer kapsoWebhookProcessedKeys.mu.Unlock()

	for existingKey, ts := range kapsoWebhookProcessedKeys.keys {
		if now.Sub(ts) > kapsoWebhookProcessedKeyTTL {
			delete(kapsoWebhookProcessedKeys.keys, existingKey)
		}
	}
	if _, ok := kapsoWebhookProcessedKeys.keys[key]; ok {
		return true
	}
	kapsoWebhookProcessedKeys.keys[key] = now
	return false
}

func isKapsoInboundMessageEvent(eventType string, event kapsoMessageEvent) bool {
	normalizedType := strings.ToLower(strings.TrimSpace(eventType))
	if normalizedType != "" && normalizedType != "whatsapp.message.received" {
		return false
	}
	direction := strings.ToLower(strings.TrimSpace(event.Message.Kapso.Direction))
	if direction != "" && direction != "inbound" {
		return false
	}
	return true
}

func normalizeDialNumber(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sendKapsoTextMessage(phoneNumberID, to, body string) error {
	apiKey := strings.TrimSpace(os.Getenv("KAPSO_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("KAPSO_API_KEY is not configured")
	}
	phoneNumberID = strings.TrimSpace(phoneNumberID)
	to = normalizeDialNumber(to)
	body = strings.TrimSpace(body)
	if phoneNumberID == "" || to == "" || body == "" {
		return fmt.Errorf("missing required data to send message")
	}

	endpoint := fmt.Sprintf("%s/%s/messages", kapsoAPIBaseURL, url.PathEscape(phoneNumberID))
	payload := kapsoSendMessagePayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text:             kapsoSendMessagePayloadText{Body: body},
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), kapsoWebhookOutboundTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encodedPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{Timeout: kapsoWebhookOutboundTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, kapsoWebhookFailureBodyLimit))
	return fmt.Errorf("kapso send failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
}

func strconvParseFloat(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}
