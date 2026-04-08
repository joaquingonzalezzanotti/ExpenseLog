package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"testing"
)

func TestParseWhatsAppExpenseTextCommand(t *testing.T) {
	parsed, err := parseWhatsAppExpenseText("/gasto 1500 Supermercado", "ars")
	if err != nil {
		t.Fatalf("parseWhatsAppExpenseText returned error: %v", err)
	}
	if parsed.Flow != "expense" {
		t.Fatalf("unexpected flow: %q", parsed.Flow)
	}
	if parsed.Amount != 1500 {
		t.Fatalf("unexpected amount: %v", parsed.Amount)
	}
	if parsed.Name != "Supermercado" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
}

func TestParseWhatsAppExpenseTextNaturalLanguage(t *testing.T) {
	parsed, err := parseWhatsAppExpenseText("gaste 2000 en el super", "ars")
	if err != nil {
		t.Fatalf("parseWhatsAppExpenseText returned error: %v", err)
	}
	if parsed.Flow != "expense" {
		t.Fatalf("unexpected flow: %q", parsed.Flow)
	}
	if parsed.Amount != 2000 {
		t.Fatalf("unexpected amount: %v", parsed.Amount)
	}
	if parsed.Name != "el super" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
}

func TestParseWhatsAppExpenseTextNaturalRefund(t *testing.T) {
	parsed, err := parseWhatsAppExpenseText("me devolvieron 1.200 en coto", "ars")
	if err != nil {
		t.Fatalf("parseWhatsAppExpenseText returned error: %v", err)
	}
	if parsed.Flow != "refund" {
		t.Fatalf("unexpected flow: %q", parsed.Flow)
	}
	if parsed.Amount != 1200 {
		t.Fatalf("unexpected amount: %v", parsed.Amount)
	}
	if parsed.Name != "coto" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
}

func TestParseWhatsAppAmountToken(t *testing.T) {
	tests := []struct {
		raw  string
		want float64
	}{
		{raw: "1500", want: 1500},
		{raw: "50.000", want: 50000},
		{raw: "66.398,96", want: 66398.96},
		{raw: "66,398.96", want: 66398.96},
		{raw: "$ 2000,50", want: 2000.50},
	}

	for _, tt := range tests {
		got, err := parseWhatsAppAmountToken(tt.raw)
		if err != nil {
			t.Fatalf("parseWhatsAppAmountToken(%q) returned error: %v", tt.raw, err)
		}
		if math.Abs(got-tt.want) > 0.0001 {
			t.Fatalf("parseWhatsAppAmountToken(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestExtractKapsoMessageEventsSupportsDataArray(t *testing.T) {
	body := []byte(`{
		"type":"whatsapp.message.received",
		"data":[
			{
				"message":{"id":"m1","type":"text","text":{"body":"hola"},"kapso":{"direction":"inbound"}},
				"conversation":{"phone_number":"5491122334455","phone_number_id":"123"}
			},
			{
				"message":{"id":"m2","type":"image","kapso":{"direction":"inbound","has_media":true}},
				"conversation":{"phone_number":"5491122334455","phone_number_id":"123"}
			}
		]
	}`)

	events, eventType := extractKapsoMessageEvents(body, "")
	if eventType != "whatsapp.message.received" {
		t.Fatalf("unexpected eventType: %q", eventType)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Message.ID != "m1" || events[1].Message.ID != "m2" {
		t.Fatalf("unexpected event ids: %#v", events)
	}
}

func TestExtractKapsoMessageEventsSupportsDirectPayload(t *testing.T) {
	body := []byte(`{
		"message":{"id":"m1","type":"text","text":{"body":"hola"},"kapso":{"direction":"inbound"}},
		"conversation":{"phone_number":"5491122334455","phone_number_id":"123"}
	}`)

	events, eventType := extractKapsoMessageEvents(body, "whatsapp.message.received")
	if eventType != "whatsapp.message.received" {
		t.Fatalf("unexpected eventType: %q", eventType)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Message.ID != "m1" {
		t.Fatalf("unexpected message id: %q", events[0].Message.ID)
	}
}

func TestIsKapsoWebhookSignatureValidAcceptsSha256Prefix(t *testing.T) {
	body := []byte(`{"ok":true}`)
	secret := "top-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !isKapsoWebhookSignatureValid(body, signature, secret) {
		t.Fatalf("expected signature to be valid")
	}
}

func TestWhatsAppEditCommandREAcceptsBracketedID(t *testing.T) {
	matches := whatsAppEditCommandRE.FindStringSubmatch("editar [c0672f2e] monto 2500")
	if len(matches) < 4 {
		t.Fatalf("expected edit command with bracketed id to match, got %d groups", len(matches))
	}
	if matches[1] != "c0672f2e" {
		t.Fatalf("unexpected captured id: %q", matches[1])
	}
	if matches[2] != "monto" {
		t.Fatalf("unexpected captured field: %q", matches[2])
	}
	if matches[3] != "2500" {
		t.Fatalf("unexpected captured value: %q", matches[3])
	}
}

func TestWhatsAppDeleteCommandREAcceptsEliminarAndBracketedID(t *testing.T) {
	matches := whatsAppDeleteCommandRE.FindStringSubmatch("eliminar [c0672f2e]")
	if len(matches) < 3 {
		t.Fatalf("expected delete command with bracketed id to match, got %d groups", len(matches))
	}
	if matches[1] != "eliminar" {
		t.Fatalf("unexpected captured verb: %q", matches[1])
	}
	if matches[2] != "c0672f2e" {
		t.Fatalf("unexpected captured id: %q", matches[2])
	}
}

func TestShortWhatsAppExpenseID(t *testing.T) {
	if got := shortWhatsAppExpenseID("1234567890"); got != "12345678" {
		t.Fatalf("unexpected short id: %q", got)
	}
	if got := shortWhatsAppExpenseID("abcd1234"); got != "abcd1234" {
		t.Fatalf("unexpected unchanged id: %q", got)
	}
}
