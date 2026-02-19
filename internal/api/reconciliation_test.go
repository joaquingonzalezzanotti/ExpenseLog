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

func TestParseTaggedFloatSupportsSanitizedFormats(t *testing.T) {
	tags := []string{"target109467.69", "before 104232.67"}
	target := parseTaggedFloat(tags, "target:")
	before := parseTaggedFloat(tags, "before:")
	if target == nil || *target != 109467.69 {
		t.Fatalf("unexpected target parse for sanitized tags: %#v", target)
	}
	if before == nil || *before != 104232.67 {
		t.Fatalf("unexpected before parse for sanitized tags: %#v", before)
	}
}

func TestHasIdempotencyTagSupportsSanitizedFormat(t *testing.T) {
	tags := []string{"reconciliation_adjustment", "idem rec-12345"}
	if !hasIdempotencyTag(tags, "rec-12345") {
		t.Fatalf("expected idempotency match for sanitized tag format")
	}
}

func TestExtractReversedReferenceSupportsSanitizedFormat(t *testing.T) {
	tags := []string{"reconciliation_reversal", "reversed 8f8a9c"}
	if ref := extractReversedReference(tags); ref != "8f8a9c" {
		t.Fatalf("expected reversed reference 8f8a9c, got %q", ref)
	}
}

func TestIsAdjustmentByFallback(t *testing.T) {
	exp := storage.Expense{
		Name:     "Ajuste conciliacion CA",
		Category: "Conciliacion",
	}
	if !isAdjustmentByFallback(exp) {
		t.Fatalf("expected fallback adjustment detection to be true")
	}
}
