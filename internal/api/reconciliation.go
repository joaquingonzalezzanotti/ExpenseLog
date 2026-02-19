package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	reconciliationCategory       = "_Conciliacion"
	reconciliationTagAdjustment  = "reconciliation_adjustment"
	reconciliationTagReversal    = "reconciliation_reversal"
	reconciliationTagReversedRef = "reversed:"
	reconciliationTagIdempotency = "idem:"
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

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), wanted) {
			return true
		}
	}
	return false
}

func firstTagWithPrefix(tags []string, prefix string) string {
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(prefix)) {
			return trimmed
		}
	}
	return ""
}

func parseTaggedFloat(tags []string, prefix string) *float64 {
	raw := firstTagWithPrefix(tags, prefix)
	if raw == "" {
		return nil
	}
	value := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func calculateCurrentCABalance(expenses []storage.Expense, currency string, now time.Time) float64 {
	balance := 0.0
	for _, exp := range expenses {
		source := strings.ToUpper(strings.TrimSpace(exp.Source))
		if source != "" && source != "CA" {
			continue
		}
		expCurrency := strings.ToLower(strings.TrimSpace(exp.Currency))
		if expCurrency != strings.ToLower(strings.TrimSpace(currency)) {
			continue
		}
		if exp.Date.After(now) {
			continue
		}
		balance += exp.Amount
	}
	return balance
}

func normalizeCurrency(cur string) string {
	cur = strings.ToLower(strings.TrimSpace(cur))
	if cur == "" {
		return "ars"
	}
	return cur
}

func buildReconciliationTags(target, previous float64, idempotencyKey, note string) []string {
	tags := []string{
		reconciliationTagAdjustment,
		fmt.Sprintf("target:%.2f", target),
		fmt.Sprintf("before:%.2f", previous),
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		tags = append(tags, reconciliationTagIdempotency+strings.TrimSpace(idempotencyKey))
	}
	note = strings.TrimSpace(note)
	if note != "" {
		tags = append(tags, "note:"+note)
	}
	return tags
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
	currency := normalizeCurrency(req.Currency)
	if req.Currency == "" {
		if cfgCur, err := h.storage.GetCurrency(userID); err == nil {
			currency = normalizeCurrency(cfgCur)
		}
	}
	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load expenses"})
		log.Printf("API ERROR: Failed to load expenses for reconciliation: %v\n", err)
		return
	}
	now := time.Now()
	currentBalance := calculateCurrentCABalance(expenses, currency, now)
	difference := req.TargetBalance - currentBalance
	if math.Abs(difference) < 0.005 {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         "noop",
			"currentBalance": currentBalance,
			"difference":     0,
			"currency":       currency,
		})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey != "" {
		idemTag := reconciliationTagIdempotency + idempotencyKey
		for _, exp := range expenses {
			if hasTag(exp.Tags, reconciliationTagAdjustment) && hasTag(exp.Tags, idemTag) {
				writeJSON(w, http.StatusOK, map[string]any{
					"status":         "duplicate",
					"expense":        exp,
					"currentBalance": currentBalance,
					"difference":     difference,
					"currency":       currency,
				})
				return
			}
		}
	}
	flow := "expense"
	if difference > 0 {
		flow = "income"
	}
	adj := storage.Expense{
		Name:     "Ajuste conciliacion CA",
		Category: reconciliationCategory,
		Amount:   difference,
		Currency: currency,
		Source:   "CA",
		Card:     "",
		Flow:     flow,
		Date:     now,
		Tags:     buildReconciliationTags(req.TargetBalance, currentBalance, idempotencyKey, req.Note),
	}
	if err := adj.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, normalizedAmount, err := normalizeFlow(adj.Flow, adj.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	adj.Flow = flow
	adj.Amount = normalizedAmount
	if err := h.storage.AddExpense(userID, adj); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to save reconciliation adjustment"})
		log.Printf("API ERROR: Failed to save reconciliation adjustment: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "applied",
		"expense":        adj,
		"currentBalance": currentBalance,
		"difference":     difference,
		"currency":       currency,
	})
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
	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load expenses"})
		return
	}
	reversedRefs := map[string]string{}
	items := make([]reconciliationHistoryItem, 0)
	for _, exp := range expenses {
		if hasTag(exp.Tags, reconciliationTagReversal) {
			ref := strings.TrimSpace(strings.TrimPrefix(firstTagWithPrefix(exp.Tags, reconciliationTagReversedRef), reconciliationTagReversedRef))
			if ref != "" {
				reversedRefs[ref] = exp.ID
			}
			items = append(items, reconciliationHistoryItem{ID: exp.ID, Date: exp.Date, Name: exp.Name, Amount: exp.Amount, Currency: exp.Currency, Type: "reversal"})
			continue
		}
		if !hasTag(exp.Tags, reconciliationTagAdjustment) {
			continue
		}
		items = append(items, reconciliationHistoryItem{
			ID:             exp.ID,
			Date:           exp.Date,
			Name:           exp.Name,
			Amount:         exp.Amount,
			Currency:       exp.Currency,
			Type:           "adjustment",
			TargetBalance:  parseTaggedFloat(exp.Tags, "target:"),
			AppBalancePrev: parseTaggedFloat(exp.Tags, "before:"),
		})
	}
	for i := range items {
		if items[i].Type != "adjustment" {
			continue
		}
		if reversedBy, ok := reversedRefs[items[i].ID]; ok {
			items[i].Reversed = true
			items[i].ReversedBy = reversedBy
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Date.After(items[j].Date)
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}
	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load expenses"})
		return
	}
	var target *storage.Expense
	for i := range expenses {
		exp := expenses[i]
		if exp.ID == id {
			target = &exp
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Reconciliation adjustment not found"})
		return
	}
	if !hasTag(target.Tags, reconciliationTagAdjustment) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Selected expense is not a reconciliation adjustment"})
		return
	}
	for _, exp := range expenses {
		if !hasTag(exp.Tags, reconciliationTagReversal) {
			continue
		}
		ref := strings.TrimSpace(strings.TrimPrefix(firstTagWithPrefix(exp.Tags, reconciliationTagReversedRef), reconciliationTagReversedRef))
		if ref == id {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Reconciliation is already reverted"})
			return
		}
	}
	reversalAmount := -target.Amount
	flow := "expense"
	if reversalAmount > 0 {
		flow = "income"
	}
	reversal := storage.Expense{
		Name:     "Reversion ajuste conciliacion CA",
		Category: reconciliationCategory,
		Amount:   reversalAmount,
		Currency: normalizeCurrency(target.Currency),
		Source:   "CA",
		Flow:     flow,
		Date:     time.Now(),
		Tags: []string{
			reconciliationTagReversal,
			reconciliationTagReversedRef + target.ID,
		},
	}
	if err := reversal.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, normalizedAmount, err := normalizeFlow(reversal.Flow, reversal.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	reversal.Flow = flow
	reversal.Amount = normalizedAmount
	if err := h.storage.AddExpense(userID, reversal); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to revert reconciliation"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "reverted", "expense": reversal})
}
