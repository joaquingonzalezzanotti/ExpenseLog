package api

import (
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func TestCalculateCurrentCABalance(t *testing.T) {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	expenses := []storage.Expense{
		{Amount: 1000, Currency: "ars", Source: "CA", Date: now.Add(-24 * time.Hour)},
		{Amount: -300, Currency: "ars", Source: "", Date: now.Add(-2 * time.Hour)},
		{Amount: 900, Currency: "ars", Source: "TARJETA", Date: now.Add(-2 * time.Hour)},
		{Amount: 700, Currency: "usd", Source: "CA", Date: now.Add(-2 * time.Hour)},
		{Amount: 50, Currency: "ars", Source: "CA", Date: now.Add(24 * time.Hour)},
	}
	got := calculateCurrentCABalance(expenses, "ars", now)
	if got != 700 {
		t.Fatalf("expected 700, got %.2f", got)
	}
}

func TestParseTaggedFloat(t *testing.T) {
	tags := []string{"reconciliation_adjustment", "target:1500.50", "before:1200.25"}
	target := parseTaggedFloat(tags, "target:")
	before := parseTaggedFloat(tags, "before:")
	if target == nil || *target != 1500.50 {
		t.Fatalf("unexpected target tag parse: %#v", target)
	}
	if before == nil || *before != 1200.25 {
		t.Fatalf("unexpected before tag parse: %#v", before)
	}
}
