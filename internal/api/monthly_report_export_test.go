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
		{Name: "Ingreso", Source: "CA", Category: "Ingresos", Amount: 10000, Flow: "income", Date: time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)},
		{Name: "Reintegro", Source: "CA", Category: "Compras", Amount: 1500, Flow: "refund", Date: time.Date(2026, time.February, 4, 12, 0, 0, 0, time.UTC)},
		{Name: "Compra debito", Source: "CA", Category: "Supermercado", Amount: -3000, Flow: "expense", Date: time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC)},
		{Name: "Compra credito", Source: "TARJETA", Category: "Supermercado", Amount: -5000, Flow: "expense", Date: time.Date(2026, time.February, 10, 18, 0, 0, 0, time.UTC)},
		{Name: "Pago tarjeta", Source: "CA", Category: "Finanzas", Amount: -2000, Flow: "expense", SystemOrigin: "card_payment_owner", Date: time.Date(2026, time.February, 20, 12, 0, 0, 0, time.UTC)},
	}

	categories := buildMonthlyReportCategoryStats(expenses)
	metrics := calculateMonthlyReportMetrics(expenses, categories)
	if metrics.TransactionCount != 5 {
		t.Fatalf("expected 5 transactions, got %d", metrics.TransactionCount)
	}
	if metrics.Income != 10000 {
		t.Fatalf("expected income 10000, got %.2f", metrics.Income)
	}
	if metrics.Expense != 5000 {
		t.Fatalf("expected expense 5000, got %.2f", metrics.Expense)
	}
	if metrics.NetBalance != 6500 {
		t.Fatalf("expected net balance 6500, got %.2f", metrics.NetBalance)
	}
	if metrics.CardPending != 3000 {
		t.Fatalf("expected card pending 3000, got %.2f", metrics.CardPending)
	}
	if metrics.TotalOutflow != 10000 {
		t.Fatalf("expected total outflow 10000, got %.2f", metrics.TotalOutflow)
	}
	if metrics.CardOutflow != 5000 {
		t.Fatalf("expected card outflow 5000, got %.2f", metrics.CardOutflow)
	}
	if metrics.ActiveDays != 4 {
		t.Fatalf("expected 4 active days, got %d", metrics.ActiveDays)
	}
	if metrics.CategoryConcentrationTop3 <= 0 {
		t.Fatalf("expected positive category concentration, got %.2f", metrics.CategoryConcentrationTop3)
	}
}

func TestBuildMonthlyReportCategoryStats(t *testing.T) {
	expenses := []storage.Expense{
		{Category: "Supermercado", Amount: -1200},
		{Category: "Supermercado", Amount: -800},
		{Category: "Transporte", Amount: -500},
		{Category: "Ingresos", Amount: 3000},
	}

	rows := buildMonthlyReportCategoryStats(expenses)
	if len(rows) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(rows))
	}
	if rows[0].Name != "Supermercado" {
		t.Fatalf("expected top category Supermercado, got %s", rows[0].Name)
	}
	if rows[0].ExpenseShare <= rows[1].ExpenseShare {
		t.Fatalf("expected first category to have greater share than second")
	}
}

func TestBuildMonthlyReportInsights(t *testing.T) {
	query := monthlyReportQuery{Year: 2026, Month: 2, Currency: "ars"}
	metrics := monthlyReportMetrics{
		TransactionCount:          4,
		SavingsRate:               -10,
		CategoryConcentrationTop3: 75,
		CardExpenseShare:          60,
		CashExpenseShare:          40,
		AvgExpenseTicket:          2000,
		MedianExpenseTicket:       1000,
		LargestExpenseName:        "Alquiler",
		LargestExpenseAmount:      5000,
		LargestExpenseDate:        time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC),
	}
	categories := []monthlyReportCategoryStat{
		{Name: "Alquiler", ExpenseTotal: 5000, ExpenseShare: 50},
	}

	insights := buildMonthlyReportInsights(metrics, categories, query)
	if len(insights) < 4 {
		t.Fatalf("expected at least 4 insights, got %d", len(insights))
	}
}
