package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	cardPaymentPaidBySelf       = "self"
	cardPaymentPaidByThirdParty = "third_party"

	systemOriginCardPaymentOwner      = "card_payment_owner"
	systemOriginCardPaymentThirdParty = "card_payment_third_party"

	cardPaymentCategory = "Tarjeta por pagar"
)

type cardPaymentRequest struct {
	Amount   float64   `json:"amount"`
	Currency string    `json:"currency"`
	Card     string    `json:"card"`
	Date     time.Time `json:"date"`
	PaidBy   string    `json:"paidBy"`
}

func normalizeCardPaymentPaidBy(raw string) (string, bool) {
	paidBy := strings.ToLower(strings.TrimSpace(raw))
	switch paidBy {
	case "", cardPaymentPaidBySelf:
		return cardPaymentPaidBySelf, true
	case cardPaymentPaidByThirdParty:
		return cardPaymentPaidByThirdParty, true
	default:
		return "", false
	}
}

func buildCardPaymentExpense(req cardPaymentRequest) (storage.Expense, error) {
	cardName := storage.SanitizeString(req.Card)
	name := "Pago tarjeta"
	flow := "expense"
	source := "CA"
	amount := req.Amount
	systemOrigin := systemOriginCardPaymentOwner

	if cardName != "" {
		name = name + " - " + cardName
	}

	if req.PaidBy == cardPaymentPaidByThirdParty {
		name = "Pago tarjeta por tercero"
		if cardName != "" {
			name = name + " - " + cardName
		}
		flow = "refund"
		source = "TARJETA"
		systemOrigin = systemOriginCardPaymentThirdParty
	}

	normalizedFlow, normalizedAmount, err := normalizeFlow(flow, amount)
	if err != nil {
		return storage.Expense{}, err
	}

	return storage.Expense{
		Name:         name,
		Category:     cardPaymentCategory,
		Amount:       normalizedAmount,
		Currency:     strings.ToLower(strings.TrimSpace(req.Currency)),
		Source:       source,
		Card:         cardName,
		Flow:         normalizedFlow,
		Date:         req.Date,
		SystemOrigin: systemOrigin,
	}, nil
}

func (h *Handler) AddCardPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req cardPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Amount must be greater than 0"})
		return
	}
	paidBy, valid := normalizeCardPaymentPaidBy(req.PaidBy)
	if !valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid paidBy value"})
		return
	}
	req.PaidBy = paidBy
	if req.Date.IsZero() {
		req.Date = time.Now()
	}
	if strings.TrimSpace(req.Currency) == "" {
		cfgCurrency, err := h.storage.GetCurrency(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to resolve currency"})
			return
		}
		req.Currency = cfgCurrency
	}

	expense, err := buildCardPaymentExpense(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := expense.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.storage.AddExpense(userID, expense); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to save card payment"})
		return
	}
	writeJSON(w, http.StatusCreated, expense)
}
