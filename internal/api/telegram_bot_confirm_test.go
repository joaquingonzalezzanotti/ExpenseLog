package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/botcore"
	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func TestConfirmBotExpenseCreatesMovementFromPendingDecision(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, _ := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	linkTelegramUserForTest(t, store, user.ID, 11223344, "confirm_tester")
	t.Setenv("UNIFIED_DECISION_ENGINE_ENABLED", "true")
	t.Setenv("EXPENSELOG_BOT_INTERNAL_SECRET", "test-secret")

	candidate := botcore.ParseCandidate{
		Channel:      botcore.ChannelTelegram,
		FlowHint:     "",
		Amount:       30000,
		Currency:     "ars",
		Counterparty: "Transferencia",
		DateTime:     time.Now().UTC(),
	}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	pending, err := store.CreateBotPendingDecision(storage.BotPendingDecision{
		UserID:          user.ID,
		Channel:         "telegram",
		SubjectKey:      "test-confirm-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		CandidateJSON:   string(candidateJSON),
		DefaultCurrency: "ars",
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateBotPendingDecision: %v", err)
	}

	req := jsonRequest(t, http.MethodPost, "/api/bot/expense/confirm", map[string]any{
		"telegram_user_id":    11223344,
		"pending_decision_id": pending.ID,
		"flow":                "income",
	})
	req.Header.Set(telegramBotSecretHeader, "test-secret")
	rec := httptest.NewRecorder()
	h.RequireBotAuth(h.ConfirmBotExpense).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", rec.Code, rec.Body.String())
	}

	expenses, err := store.GetAllExpenses(user.ID)
	if err != nil {
		t.Fatalf("GetAllExpenses: %v", err)
	}
	if len(expenses) == 0 {
		t.Fatalf("expected at least one expense after confirmation")
	}
}
