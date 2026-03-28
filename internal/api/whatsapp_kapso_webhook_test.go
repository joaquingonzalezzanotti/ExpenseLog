package api

import (
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
