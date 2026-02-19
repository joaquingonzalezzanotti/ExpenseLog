package api

import (
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func TestBuildHistoryItemsIncludesAdjustmentAndReversal(t *testing.T) {
	created := time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC)
	reverted := created.Add(2 * time.Hour)
	target := 109467.69
	before := 104232.67

	items := buildHistoryItems([]storage.ReconciliationRecord{
		{
			ID:                  "rec-1",
			AdjustmentExpenseID: "adj-1",
			ReversalExpenseID:   "rev-1",
			TargetBalance:       &target,
			AppBalanceBefore:    &before,
			DeltaAmount:         5235.02,
			Currency:            "ars",
			Status:              "reverted",
			CreatedAt:           created,
			RevertedAt:          &reverted,
		},
	})

	if len(items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(items))
	}
	if items[0].Type != "reversal" || items[0].ID != "rev-1" {
		t.Fatalf("expected first item to be reversal rev-1, got %s/%s", items[0].Type, items[0].ID)
	}
	if items[1].Type != "adjustment" || items[1].ID != "adj-1" {
		t.Fatalf("expected second item to be adjustment adj-1, got %s/%s", items[1].Type, items[1].ID)
	}
	if !items[1].Reversed || items[1].ReversedBy != "rev-1" {
		t.Fatalf("expected adjustment to be marked reversed by rev-1")
	}
	if items[1].TargetBalance == nil || *items[1].TargetBalance != target {
		t.Fatalf("expected target balance %.2f, got %#v", target, items[1].TargetBalance)
	}
	if items[1].AppBalancePrev == nil || *items[1].AppBalancePrev != before {
		t.Fatalf("expected app balance before %.2f, got %#v", before, items[1].AppBalancePrev)
	}
}

func TestBuildHistoryItemsWithoutReversal(t *testing.T) {
	created := time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC)
	items := buildHistoryItems([]storage.ReconciliationRecord{
		{
			ID:                  "rec-1",
			AdjustmentExpenseID: "adj-1",
			DeltaAmount:         -1200,
			Currency:            "ars",
			Status:              "applied",
			CreatedAt:           created,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].Type != "adjustment" || items[0].Reversed {
		t.Fatalf("expected single adjustment item not reversed, got type=%s reversed=%v", items[0].Type, items[0].Reversed)
	}
}
