package api

import (
	"testing"
	"time"
)

func TestNormalizeCardPaymentPaidBy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "default-self", input: "", want: cardPaymentPaidBySelf, valid: true},
		{name: "self", input: "self", want: cardPaymentPaidBySelf, valid: true},
		{name: "third-party", input: "third_party", want: cardPaymentPaidByThirdParty, valid: true},
		{name: "invalid", input: "other", want: "", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := normalizeCardPaymentPaidBy(tc.input)
			if valid != tc.valid {
				t.Fatalf("valid mismatch: got %v want %v", valid, tc.valid)
			}
			if got != tc.want {
				t.Fatalf("paidBy mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestBuildCardPaymentExpenseSelf(t *testing.T) {
	req := cardPaymentRequest{
		Amount:   15000,
		Currency: "ars",
		Card:     "Visa Banco",
		Date:     time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		PaidBy:   cardPaymentPaidBySelf,
	}

	expense, err := buildCardPaymentExpense(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expense.Source != "CA" {
		t.Fatalf("expected source CA, got %q", expense.Source)
	}
	if expense.Flow != "expense" {
		t.Fatalf("expected flow expense, got %q", expense.Flow)
	}
	if expense.Amount >= 0 {
		t.Fatalf("expected negative amount, got %.2f", expense.Amount)
	}
	if expense.SystemOrigin != systemOriginCardPaymentOwner {
		t.Fatalf("expected owner system origin, got %q", expense.SystemOrigin)
	}
}

func TestBuildCardPaymentExpenseThirdParty(t *testing.T) {
	req := cardPaymentRequest{
		Amount:   10000,
		Currency: "ars",
		Card:     "Master",
		Date:     time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		PaidBy:   cardPaymentPaidByThirdParty,
	}

	expense, err := buildCardPaymentExpense(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expense.Source != "TARJETA" {
		t.Fatalf("expected source TARJETA, got %q", expense.Source)
	}
	if expense.Flow != "refund" {
		t.Fatalf("expected flow refund, got %q", expense.Flow)
	}
	if expense.Amount <= 0 {
		t.Fatalf("expected positive amount, got %.2f", expense.Amount)
	}
	if expense.SystemOrigin != systemOriginCardPaymentThirdParty {
		t.Fatalf("expected third-party system origin, got %q", expense.SystemOrigin)
	}
}
