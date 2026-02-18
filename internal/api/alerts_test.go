package api

import (
	"math"
	"testing"
	"time"

	"github.com/tanq16/expenseowl/internal/storage"
)

func mkExpense(name string, amount float64, date time.Time, recurringID string) storage.Expense {
	return storage.Expense{
		Name:        name,
		Amount:      amount,
		Currency:    "ars",
		Source:      "CA",
		Date:        date,
		RecurringID: recurringID,
	}
}

func dateUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestComputeLiquidityAlerts_CaseOutsideWindow_NoAlert(t *testing.T) {
	now := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 20000, dateUTC(2026, time.February, 28), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 0 {
		t.Fatalf("expected no alerts, got %d", resp.AlertCount)
	}
	if !almostEqual(resp.BalanceNow, 20000) {
		t.Fatalf("expected balanceNow 20000, got %.2f", resp.BalanceNow)
	}
}

func TestComputeLiquidityAlerts_CaseSevenDays_Info(t *testing.T) {
	now := time.Date(2026, time.March, 3, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 20000, dateUTC(2026, time.March, 2), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 1 {
		t.Fatalf("expected 1 alert, got %d", resp.AlertCount)
	}
	item := resp.Alerts[0]
	if item.Severity != "info" || item.Kind != "preview_7d" {
		t.Fatalf("expected info/preview_7d, got %s/%s", item.Severity, item.Kind)
	}
	if !almostEqual(item.BalanceAfter, 0) {
		t.Fatalf("expected projected balance 0, got %.2f", item.BalanceAfter)
	}
}

func TestComputeLiquidityAlerts_CaseSevenDays_NegativeStillInfo(t *testing.T) {
	now := time.Date(2026, time.March, 3, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 15000, dateUTC(2026, time.March, 2), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 1 {
		t.Fatalf("expected 1 alert, got %d", resp.AlertCount)
	}
	item := resp.Alerts[0]
	if item.Severity != "info" || item.Kind != "preview_7d" {
		t.Fatalf("expected info/preview_7d, got %s/%s", item.Severity, item.Kind)
	}
	if !almostEqual(item.BalanceAfter, -5000) {
		t.Fatalf("expected projected balance -5000, got %.2f", item.BalanceAfter)
	}
}

func TestComputeLiquidityAlerts_CaseFourDays_CriticalRisk(t *testing.T) {
	now := time.Date(2026, time.March, 6, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 15000, dateUTC(2026, time.March, 5), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.CriticalCount != 1 {
		t.Fatalf("expected 1 critical alert, got %d", resp.CriticalCount)
	}
	item := resp.Alerts[0]
	if item.Severity != "critical" || item.Kind != "risk_4d" {
		t.Fatalf("expected critical/risk_4d, got %s/%s", item.Severity, item.Kind)
	}
}

func TestComputeLiquidityAlerts_CaseFourDays_MonitoringInfo(t *testing.T) {
	now := time.Date(2026, time.March, 6, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 25000, dateUTC(2026, time.March, 5), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 1 {
		t.Fatalf("expected 1 alert, got %d", resp.AlertCount)
	}
	item := resp.Alerts[0]
	if item.Severity != "info" || item.Kind != "monitor_4d" {
		t.Fatalf("expected info/monitor_4d, got %s/%s", item.Severity, item.Kind)
	}
}

func TestComputeLiquidityAlerts_CaseDueDay_InformativeOnly(t *testing.T) {
	now := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 10000, dateUTC(2026, time.March, 9), ""),
		mkExpense("Servicio", -20000, dateUTC(2026, time.March, 10), "rec-1"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 1 {
		t.Fatalf("expected 1 due alert, got %d", resp.AlertCount)
	}
	item := resp.Alerts[0]
	if item.Severity != "info" || item.Kind != "due" {
		t.Fatalf("expected info/due, got %s/%s", item.Severity, item.Kind)
	}
}

func TestComputeLiquidityAlerts_CaseSequentialRecurring_SecondBecomesRisk(t *testing.T) {
	now := time.Date(2026, time.March, 6, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 15000, dateUTC(2026, time.March, 5), ""),
		mkExpense("Cuota A", -8000, dateUTC(2026, time.March, 9), "rec-a"),
		mkExpense("Cuota B", -8000, dateUTC(2026, time.March, 10), "rec-b"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 2 {
		t.Fatalf("expected 2 alerts, got %d", resp.AlertCount)
	}
	if resp.Alerts[0].Name != "Cuota B" || resp.Alerts[0].Severity != "critical" {
		t.Fatalf("expected critical alert for second recurring first in list, got %s/%s", resp.Alerts[0].Name, resp.Alerts[0].Severity)
	}
}

func TestComputeLiquidityAlerts_TimezoneDateShift_NoFalseOneDayEarlier(t *testing.T) {
	now := time.Date(2026, time.February, 18, 9, 0, 0, 0, time.Local)
	expenses := []storage.Expense{
		mkExpense("Saldo previo", 104232.67, dateUTC(2026, time.February, 17), ""),
		// Stored as UTC midnight on 20th; must still be treated as due 20th calendar-day.
		mkExpense("Pruebaa", -120000, time.Date(2026, time.February, 20, 0, 0, 0, 0, time.UTC), "rec-20"),
	}

	resp := computeLiquidityAlerts(expenses, "ars", 7, now)
	if resp.AlertCount != 1 {
		t.Fatalf("expected 1 alert, got %d", resp.AlertCount)
	}
	if resp.Alerts[0].DaysUntil != 2 {
		t.Fatalf("expected daysUntil 2 for due date 20/02 from 18/02, got %d", resp.Alerts[0].DaysUntil)
	}
}
