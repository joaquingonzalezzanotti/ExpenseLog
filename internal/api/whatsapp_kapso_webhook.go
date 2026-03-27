package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
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
)

var (
	whatsAppExpenseCommandRE = regexp.MustCompile(`(?i)^/?(gasto|ingreso|reintegro|refund)\s+([0-9]+(?:[.,][0-9]{1,2})?)\s*(.*)$`)
	whatsAppLinkCommandRE    = regexp.MustCompile(`(?i)^/?vincular\s+([a-z0-9\-\s]+)$`)
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
	ID    string                   `json:"id"`
	From  string                   `json:"from"`
	Type  string                   `json:"type"`
	Text  kapsoWebhookMessageText  `json:"text"`
	Kapso kapsoWebhookMessageKapso `json:"kapso"`
}

type kapsoWebhookMessageText struct {
	Body string `json:"body"`
}

type kapsoWebhookMessageKapso struct {
	Direction string `json:"direction"`
	Content   string `json:"content"`
	HasMedia  bool   `json:"has_media"`
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
		reply := "Recibi tu archivo. Por ahora registra gastos por texto con: /gasto 1500 Supermercado"
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
	matches := whatsAppExpenseCommandRE.FindStringSubmatch(text)
	if len(matches) < 4 {
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_format")
	}

	flowToken := strings.ToLower(strings.TrimSpace(matches[1]))
	amountToken := strings.ReplaceAll(strings.TrimSpace(matches[2]), ",", ".")
	description := storage.SanitizeString(strings.TrimSpace(matches[3]))

	var flow string
	switch flowToken {
	case "gasto":
		flow = "expense"
	case "ingreso":
		flow = "income"
	case "reintegro", "refund":
		flow = "refund"
	default:
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_flow")
	}

	amount, parseErr := strconvParseFloat(amountToken)
	if parseErr != nil || amount <= 0 {
		return whatsAppParsedExpense{}, fmt.Errorf("invalid_amount")
	}
	amount = math.Abs(amount)

	category := "Varios"
	if flow == "income" || flow == "refund" {
		category = "Ingresos"
	}
	name := description
	if name == "" {
		if flow == "expense" {
			name = "Movimiento WhatsApp"
		} else {
			name = "Ingreso WhatsApp"
		}
	}

	return whatsAppParsedExpense{
		Flow:     flow,
		Amount:   amount,
		Name:     name,
		Category: category,
		Currency: strings.ToLower(strings.TrimSpace(defaultCurrency)),
	}, nil
}

func (h *Handler) createExpenseFromWhatsAppText(userID, text string) string {
	defaultCurrency, err := h.storage.GetCurrency(userID)
	if err != nil || strings.TrimSpace(defaultCurrency) == "" {
		defaultCurrency = "ars"
	}

	parsed, parseErr := parseWhatsAppExpenseText(text, defaultCurrency)
	if parseErr != nil {
		if parseErr.Error() == "help" {
			return "Comandos: /gasto 1500 Supermercado | /ingreso 50000 Sueldo | /reintegro 1200 Devolucion"
		}
		return "No pude interpretar el mensaje. Usa: /gasto 1500 Supermercado"
	}

	flow, adjustedAmount, err := normalizeFlow(parsed.Flow, parsed.Amount)
	if err != nil {
		return "No pude interpretar el tipo de movimiento. Usa /gasto, /ingreso o /reintegro."
	}
	expense := storage.Expense{
		ID:           uuid.New().String(),
		Flow:         flow,
		Name:         parsed.Name,
		Category:     parsed.Category,
		Amount:       adjustedAmount,
		Currency:     parsed.Currency,
		Source:       "CA",
		Tags:         uniqueTags([]string{"whatsapp_bot"}),
		SystemOrigin: "whatsapp_bot",
		Date:         time.Now().UTC(),
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
