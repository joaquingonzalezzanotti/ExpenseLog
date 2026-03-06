package api

import (
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func TestFilterExpensesForMonthlyReport(t *testing.T) {
	query := monthlyReportQuery{
		Year:     2026,
		Month:    2,
		Currency: "ars",
		Start:    time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
	}

	expenses := []storage.Expense{
		{Name: "old", Currency: "ars", Date: time.Date(2026, time.January, 31, 23, 0, 0, 0, time.UTC), Amount: -100},
		{Name: "usd", Currency: "usd", Date: time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC), Amount: -100},
		{Name: "late", Currency: "ars", Date: time.Date(2026, time.February, 20, 12, 0, 0, 0, time.UTC), Amount: -50},
		{Name: "early", Currency: "ars", Date: time.Date(2026, time.February, 5, 12, 0, 0, 0, time.UTC), Amount: 200},
	}

	filtered := filterExpensesForMonthlyReport(expenses, query)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(filtered))
	}
	if filtered[0].Name != "early" || filtered[1].Name != "late" {
		t.Fatalf("unexpected order after filtering: %s, %s", filtered[0].Name, filtered[1].Name)
	}
}

func TestCalculateMonthlyReportMetrics(t *testing.T) {
	expenses := []storage.Expense{
		{Name: "Ingreso", Source: "CA", Amount: 10000, Flow: "income"},
		{Name: "Compra debito", Source: "CA", Amount: -3000, Flow: "expense"},
		{Name: "Compra credito", Source: "TARJETA", Amount: -5000, Flow: "expense"},
		{Name: "Pago tarjeta", Source: "CA", Amount: -2000, Flow: "expense", SystemOrigin: "card_payment_owner"},
	}

	metrics := calculateMonthlyReportMetrics(expenses)
	if metrics.TransactionCount != 4 {
		t.Fatalf("expected 4 transactions, got %d", metrics.TransactionCount)
	}
	if metrics.Income != 10000 {
		t.Fatalf("expected income 10000, got %.2f", metrics.Income)
	}
	if metrics.Expense != 5000 {
		t.Fatalf("expected expense 5000, got %.2f", metrics.Expense)
	}
	if metrics.NetBalance != 5000 {
		t.Fatalf("expected net balance 5000, got %.2f", metrics.NetBalance)
	}
	if metrics.CardPending != 3000 {
		t.Fatalf("expected card pending 3000, got %.2f", metrics.CardPending)
	}
}
