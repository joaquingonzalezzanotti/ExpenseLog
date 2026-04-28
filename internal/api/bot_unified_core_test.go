package api

import (
	"encoding/json"
	"testing"
)

func TestExtractTelegramAccountHints(t *testing.T) {
	raw := map[string]any{
		"from_account": "BANCOR - CA 9800",
		"to": map[string]any{
			"account": "Santander - CA 8536",
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal source meta: %v", err)
	}
	sourceLast4, destinationLast4 := extractTelegramAccountHints(encoded)
	if sourceLast4 != "9800" {
		t.Fatalf("expected source last4=9800, got %q", sourceLast4)
	}
	if destinationLast4 != "8536" {
		t.Fatalf("expected destination last4=8536, got %q", destinationLast4)
	}
}

func TestExtractTelegramAccountHintsUnknownKeys(t *testing.T) {
	raw := map[string]any{
		"foo": "account 1111",
		"bar": "account 2222",
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal source meta: %v", err)
	}
	sourceLast4, destinationLast4 := extractTelegramAccountHints(encoded)
	if sourceLast4 != "" || destinationLast4 != "" {
		t.Fatalf("expected empty hints for unknown keys, got src=%q dst=%q", sourceLast4, destinationLast4)
	}
}

func TestExtractTelegramBatchPayloads(t *testing.T) {
	sourceMeta, err := json.Marshal(map[string]any{
		"batch_items": []map[string]any{
			{
				"type":     "expense",
				"amount":   350000,
				"motive":   "supermercado",
				"currency": "ars",
			},
			{
				"type":   "income",
				"amount": 150000,
				"motive": "ingreso",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal source meta: %v", err)
	}
	payload := botExpensePayload{
		TelegramUserID: 101,
		Currency:       "ars",
		SourceMeta:     sourceMeta,
	}
	items := extractTelegramBatchPayloads(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 batch items, got %d", len(items))
	}
	if items[0].TelegramUserID != 101 || items[1].TelegramUserID != 101 {
		t.Fatalf("expected telegram user id propagated")
	}
	if items[1].Currency != "ars" {
		t.Fatalf("expected default currency propagated to second item, got %q", items[1].Currency)
	}
}
