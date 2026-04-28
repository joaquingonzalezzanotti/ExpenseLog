package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
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

func TestHandleKapsoWhatsAppWebhookRejectsWhenSecretMissing(t *testing.T) {
	t.Setenv("KAPSO_WEBHOOK_SECRET", "")
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/kapso/whatsapp", strings.NewReader(`{"ok":true}`))
	rec := httptest.NewRecorder()

	h.HandleKapsoWhatsAppWebhook(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
}

func TestParseAndValidateKapsoMediaURL(t *testing.T) {
	t.Setenv("KAPSO_MEDIA_ALLOWED_HOSTS", "")
	if _, err := parseAndValidateKapsoMediaURL("https://api.kapso.ai/file/1"); err != nil {
		t.Fatalf("expected trusted kapso host to be accepted: %v", err)
	}
	if _, err := parseAndValidateKapsoMediaURL("https://cdn.kapso.ai/media/abc"); err != nil {
		t.Fatalf("expected kapso subdomain to be accepted: %v", err)
	}
	if _, err := parseAndValidateKapsoMediaURL("http://api.kapso.ai/file/1"); err == nil {
		t.Fatalf("expected http scheme to be rejected")
	}
	if _, err := parseAndValidateKapsoMediaURL("https://evil.example.com/media"); err == nil {
		t.Fatalf("expected non-kapso host to be rejected")
	}
}

func TestParseAndValidateKapsoMediaURLRespectsHostAllowlist(t *testing.T) {
	t.Setenv("KAPSO_MEDIA_ALLOWED_HOSTS", "media.example.com,*.example.net")
	if _, err := parseAndValidateKapsoMediaURL("https://media.example.com/file"); err != nil {
		t.Fatalf("expected explicit host allowlist to be accepted: %v", err)
	}
	if _, err := parseAndValidateKapsoMediaURL("https://cdn.example.net/file"); err != nil {
		t.Fatalf("expected wildcard allowlist host to be accepted: %v", err)
	}
	if _, err := parseAndValidateKapsoMediaURL("https://api.kapso.ai/file"); err == nil {
		t.Fatalf("expected non-allowlisted host to be rejected when custom allowlist is set")
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

func TestFormatWhatsAppRecentExpensesSummaryFiltersFuture(t *testing.T) {
	now := time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC)
	expenses := []storage.Expense{
		{
			ID:       "future-1111-aaaa",
			Flow:     "income",
			Amount:   30000,
			Currency: "ars",
			Name:     "Sueldo futuro",
			Date:     now.Add(24 * time.Hour),
		},
		{
			ID:       "c0672f2e-aaaa-bbbb",
			Flow:     "income",
			Amount:   30000,
			Currency: "ars",
			Name:     "Sueldo Bacar JP",
			Date:     now.Add(-1 * time.Hour),
		},
		{
			ID:       "55d4065e-aaaa-bbbb",
			Flow:     "income",
			Amount:   30000,
			Currency: "ars",
			Name:     "Sueldo Bacar JP",
			Date:     now.Add(-2 * time.Hour),
		},
	}

	got := formatWhatsAppRecentExpensesSummary(expenses, now)

	if strings.Contains(got, "future-11") {
		t.Fatalf("summary should exclude future expense, got: %q", got)
	}
	if !strings.Contains(got, "1) c0672f2e") {
		t.Fatalf("summary should keep latest non-future movement first, got: %q", got)
	}
	if !strings.Contains(got, "Ingreso 30000.00 ARS") {
		t.Fatalf("summary should use user-facing flow labels, got: %q", got)
	}
	if strings.Contains(got, "INCOME") || strings.Contains(got, "EXPENSE") || strings.Contains(got, "REFUND") {
		t.Fatalf("summary should not expose technical flow tokens, got: %q", got)
	}
	if !strings.Contains(got, "Acciones rapidas (usa el ID del listado):") {
		t.Fatalf("summary should include quick actions heading, got: %q", got)
	}
	if !strings.Contains(got, "- editar [c0672f2e] monto 2500") {
		t.Fatalf("summary should include bracketed edit example, got: %q", got)
	}
}

func TestFormatWhatsAppRecentExpensesSummaryOnlyFuture(t *testing.T) {
	now := time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC)
	expenses := []storage.Expense{
		{
			ID:       "future-1111-aaaa",
			Flow:     "income",
			Amount:   30000,
			Currency: "ars",
			Name:     "Sueldo futuro",
			Date:     now.Add(2 * time.Hour),
		},
	}

	got := formatWhatsAppRecentExpensesSummary(expenses, now)
	want := "No hay movimientos con fecha hasta hoy. Solo veo movimientos futuros."
	if got != want {
		t.Fatalf("unexpected summary. got=%q want=%q", got, want)
	}
}

func TestWhatsAppFlowLabel(t *testing.T) {
	cases := []struct {
		flow   string
		amount float64
		want   string
	}{
		{flow: "income", amount: 10, want: "Ingreso"},
		{flow: "expense", amount: -10, want: "Gasto"},
		{flow: "refund", amount: 10, want: "Reintegro"},
		{flow: "", amount: -1, want: "Gasto"},
		{flow: "", amount: 1, want: "Ingreso"},
		{flow: "", amount: 0, want: "Movimiento"},
	}

	for _, tc := range cases {
		if got := whatsAppFlowLabel(tc.flow, tc.amount); got != tc.want {
			t.Fatalf("whatsAppFlowLabel(%q, %v)=%q want=%q", tc.flow, tc.amount, got, tc.want)
		}
	}
}

func TestSummarizeWhatsAppBatchResult(t *testing.T) {
	message := summarizeWhatsAppBatchResult([]storage.Expense{
		{Flow: "expense", Amount: -350000, Currency: "ars"},
		{Flow: "expense", Amount: -15000, Currency: "ars"},
		{Flow: "expense", Amount: -250000, Currency: "ars"},
		{Flow: "income", Amount: 150000, Currency: "ars"},
	})
	if !strings.Contains(message, "cargue 4 movimientos") {
		t.Fatalf("expected movement count summary, got: %q", message)
	}
	if !strings.Contains(message, "Total ingresado: 150000.00 ARS") {
		t.Fatalf("expected income total in summary, got: %q", message)
	}
	if !strings.Contains(message, "Total gastado: 615000.00 ARS") {
		t.Fatalf("expected expense total in summary, got: %q", message)
	}
	if !strings.Contains(message, "Saldo neto: -465000.00 ARS") {
		t.Fatalf("expected net balance in summary, got: %q", message)
	}
}

func TestMapWhatsAppFlowConfirmation(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "ingreso", want: "income", ok: true},
		{input: "income", want: "income", ok: true},
		{input: "gasto", want: "expense", ok: true},
		{input: "egreso", want: "expense", ok: true},
		{input: "maybe", want: "", ok: false},
	}
	for _, tt := range tests {
		got, ok := mapWhatsAppFlowConfirmation(tt.input)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("mapWhatsAppFlowConfirmation(%q) = (%q,%v), want (%q,%v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}
