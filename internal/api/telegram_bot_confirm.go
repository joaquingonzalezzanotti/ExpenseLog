package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/botcore"
)

type botConfirmExpenseRequest struct {
	TelegramUserID    int64  `json:"telegram_user_id"`
	PendingDecisionID string `json:"pending_decision_id"`
	Flow              string `json:"flow"`
}

func (h *Handler) ConfirmBotExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeBotError(w, http.StatusMethodNotAllowed, "Method not allowed", "method_not_allowed")
		return
	}
	if !isUnifiedDecisionEngineEnabled() {
		writeBotError(w, http.StatusPreconditionFailed, "Feature disabled", "feature_disabled")
		return
	}

	var payload botConfirmExpenseRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeBotError(w, http.StatusBadRequest, "Invalid request body", "invalid_payload")
		return
	}
	if payload.TelegramUserID <= 0 || strings.TrimSpace(payload.PendingDecisionID) == "" {
		writeBotError(w, http.StatusBadRequest, "telegram_user_id and pending_decision_id are required", "invalid_payload")
		return
	}
	flow := strings.ToLower(strings.TrimSpace(payload.Flow))
	if flow != "income" && flow != "expense" {
		writeBotError(w, http.StatusBadRequest, "flow must be income or expense", "invalid_payload")
		return
	}

	link, err := h.storage.GetTelegramUserLinkByTelegramUserID(payload.TelegramUserID)
	if err == sql.ErrNoRows {
		writeBotError(w, http.StatusNotFound, "Telegram user not linked", "not_linked")
		return
	}
	if err != nil {
		writeBotError(w, http.StatusInternalServerError, "No se pudo resolver el usuario de Telegram", "internal_error")
		return
	}

	pending, err := h.storage.GetBotPendingDecisionByID(payload.PendingDecisionID)
	if err == sql.ErrNoRows {
		writeBotError(w, http.StatusNotFound, "Pending decision not found", "not_found")
		return
	}
	if err != nil {
		writeBotError(w, http.StatusInternalServerError, "No se pudo obtener la decision pendiente", "internal_error")
		return
	}
	if pending.UserID != link.UserID || pending.Channel != "telegram" {
		writeBotError(w, http.StatusForbidden, "Pending decision does not belong to this user/channel", "forbidden")
		return
	}

	now := time.Now().UTC()
	resolvedPending, err := h.storage.ResolveBotPendingDecision(payload.PendingDecisionID, now)
	if err == sql.ErrNoRows {
		writeBotError(w, http.StatusConflict, "Pending decision expired or already resolved", "expired_or_resolved")
		return
	}
	if err != nil {
		writeBotError(w, http.StatusInternalServerError, "No se pudo resolver la decision pendiente", "internal_error")
		return
	}

	var candidate botcore.ParseCandidate
	if err := json.Unmarshal([]byte(resolvedPending.CandidateJSON), &candidate); err != nil {
		writeBotError(w, http.StatusInternalServerError, "Pending candidate is invalid", "internal_error")
		return
	}
	candidate.FlowHint = flow

	decision, err := h.decideFromCandidate(link.UserID, candidate, resolvedPending.DefaultCurrency, now)
	if err != nil {
		writeBotError(w, http.StatusBadRequest, err.Error(), "invalid_payload")
		return
	}
	decision.Ambiguous = false
	decision.Reasons = append(decision.Reasons, "user_confirmed_flow")
	h.recordBotDecisionEvent(link.UserID, candidate, decision, now)

	expense := decisionToExpense(decision, []string{"telegram_bot", "manual_confirmation"}, "telegram_bot")
	if err := expense.Validate(); err != nil {
		writeBotError(w, http.StatusBadRequest, err.Error(), "invalid_payload")
		return
	}
	if err := h.storage.AddExpense(link.UserID, expense); err != nil {
		writeBotError(w, http.StatusInternalServerError, "No se pudo crear la transaccion", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, botExpenseResponse{
		TransactionID: expense.ID,
		URL:           "/app/table",
	})
}
