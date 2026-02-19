package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

type reconcileRequest struct {
	TargetBalance float64 `json:"targetBalance"`
	Currency      string  `json:"currency"`
	Note          string  `json:"note"`
}

type reconciliationHistoryItem struct {
	ID             string    `json:"id"`
	Date           time.Time `json:"date"`
	Name           string    `json:"name"`
	Amount         float64   `json:"amount"`
	Currency       string    `json:"currency"`
	Type           string    `json:"type"`
	TargetBalance  *float64  `json:"targetBalance,omitempty"`
	AppBalancePrev *float64  `json:"appBalanceBefore,omitempty"`
	Reversed       bool      `json:"reversed"`
	ReversedBy     string    `json:"reversedBy,omitempty"`
}

func (h *Handler) ReconcileBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req reconcileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	result, err := h.storage.ApplyReconciliation(userID, storage.ReconciliationApplyInput{
		TargetBalance:  req.TargetBalance,
		Currency:       req.Currency,
		Note:           req.Note,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		Now:            time.Now(),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to apply reconciliation"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         result.Status,
		"expense":        result.Expense,
		"currentBalance": result.CurrentBalance,
		"difference":     result.Difference,
		"currency":       result.Currency,
	})
}

func buildHistoryItems(records []storage.ReconciliationRecord) []reconciliationHistoryItem {
	items := make([]reconciliationHistoryItem, 0, len(records)*2)
	for _, rec := range records {
		adjustment := reconciliationHistoryItem{
			ID:             rec.AdjustmentExpenseID,
			Date:           rec.CreatedAt,
			Name:           "Ajuste conciliacion CA",
			Amount:         rec.DeltaAmount,
			Currency:       rec.Currency,
			Type:           "adjustment",
			TargetBalance:  rec.TargetBalance,
			AppBalancePrev: rec.AppBalanceBefore,
			Reversed:       strings.EqualFold(rec.Status, "reverted") || strings.TrimSpace(rec.ReversalExpenseID) != "",
			ReversedBy:     rec.ReversalExpenseID,
		}
		items = append(items, adjustment)

		if strings.TrimSpace(rec.ReversalExpenseID) != "" {
			reversalDate := rec.CreatedAt
			if rec.RevertedAt != nil {
				reversalDate = *rec.RevertedAt
			}
			items = append(items, reconciliationHistoryItem{
				ID:       rec.ReversalExpenseID,
				Date:     reversalDate,
				Name:     "Reversion ajuste conciliacion CA",
				Amount:   -rec.DeltaAmount,
				Currency: rec.Currency,
				Type:     "reversal",
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Date.After(items[j].Date)
	})
	return items
}

func (h *Handler) GetReconciliationHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	records, err := h.storage.GetReconciliationHistory(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load reconciliation history"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": buildHistoryItems(records)})
}

func (h *Handler) RevertReconciliation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	adjustmentExpenseID := strings.TrimSpace(r.URL.Query().Get("id"))
	if adjustmentExpenseID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}

	reversal, err := h.storage.RevertReconciliation(userID, adjustmentExpenseID, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrReconciliationNotFound):
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Reconciliation adjustment not found"})
		case errors.Is(err, storage.ErrReconciliationAlreadyReverted):
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Reconciliation is already reverted"})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to revert reconciliation"})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "reverted", "expense": reversal})
}
