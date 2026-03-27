package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
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
	ID        string                   `json:"id"`
	From      string                   `json:"from"`
	Type      string                   `json:"type"`
	Text      kapsoWebhookMessageText  `json:"text"`
	Kapso     kapsoWebhookMessageKapso `json:"kapso"`
	RawFields map[string]any           `json:"-"`
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

		phoneNumberID := strings.TrimSpace(event.PhoneNumberID)
		if phoneNumberID == "" {
			phoneNumberID = strings.TrimSpace(event.Conversation.PhoneNumberID)
		}
		if phoneNumberID == "" {
			log.Printf("WHATSAPP KAPSO: ignoring inbound message without phone_number_id")
			continue
		}

		recipient := normalizeDialNumber(event.Conversation.PhoneNumber)
		if recipient == "" {
			recipient = normalizeDialNumber(event.Message.From)
		}
		if recipient == "" {
			log.Printf("WHATSAPP KAPSO: ignoring inbound message without recipient number")
			continue
		}

		replyText := buildKapsoAutoReplyText(event.Message)
		if strings.TrimSpace(replyText) == "" {
			continue
		}

		if err := sendKapsoTextMessage(phoneNumberID, recipient, replyText); err != nil {
			log.Printf("WHATSAPP KAPSO: failed sending auto-reply to %s: %v", recipient, err)
		}
	}
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

func kapsoAutoReplyEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("WHATSAPP_KAPSO_AUTO_REPLY_ENABLED")))
	switch raw {
	case "", "1", "true", "yes", "si", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func buildKapsoAutoReplyText(message kapsoWebhookMessage) string {
	if !kapsoAutoReplyEnabled() {
		return ""
	}
	customText := strings.TrimSpace(os.Getenv("WHATSAPP_KAPSO_AUTO_REPLY_TEXT"))
	textBody := strings.TrimSpace(message.Text.Body)
	if textBody == "" {
		textBody = strings.TrimSpace(message.Kapso.Content)
	}
	normalized := strings.ToLower(strings.TrimSpace(textBody))

	if message.Kapso.HasMedia || (strings.TrimSpace(message.Type) != "" && !strings.EqualFold(strings.TrimSpace(message.Type), "text")) {
		return "Recibi tu archivo en ExpenseLog. Todavia no tengo procesamiento automatico de imagen/PDF por WhatsApp en esta instancia."
	}
	if normalized == "ping" || normalized == "/ping" {
		return "ExpenseLog conectado por WhatsApp. Estado: OK."
	}
	if normalized == "ayuda" || normalized == "/ayuda" || normalized == "help" || normalized == "/help" {
		return "Canal activo. Por ahora este bot responde mensajes y permite validar la integracion webhook. El parser automatico de gastos por WhatsApp queda como siguiente paso."
	}
	if customText != "" {
		return customText
	}
	return "Mensaje recibido en ExpenseLog. Si quieres probar conectividad, envia /ping."
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
