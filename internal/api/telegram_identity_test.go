package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func linkTelegramUserForTest(t *testing.T, s storage.Storage, userID string, telegramUserID int64, username string) {
	t.Helper()
	now := time.Now().UTC()
	codeHash := "test-code-hash-" + strconv.FormatInt(now.UnixNano(), 10)
	if _, err := s.CreateTelegramLinkCode(userID, codeHash, now.Add(15*time.Minute), now); err != nil {
		t.Fatalf("create telegram link code: %v", err)
	}
	if _, err := s.ConsumeTelegramLinkCode(codeHash, telegramUserID, username, now.Add(time.Second)); err != nil {
		t.Fatalf("consume telegram link code: %v", err)
	}
}

func TestTelegramIdentityFeatureFlagDisabled(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	_, sessionID := createUserAndSession(t, store)
	t.Setenv("TELEGRAM_IDENTITY_V2_ENABLED", "false")

	req := httptest.NewRequest(http.MethodGet, "/api/telegram/identity", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.TelegramIdentity).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when flag is disabled, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTelegramIdentityPutRequiresPremiumAndLinked(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	t.Setenv("TELEGRAM_IDENTITY_V2_ENABLED", "true")

	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	req := jsonRequest(t, http.MethodPut, "/api/telegram/identity", map[string]any{
		"aliases": []map[string]any{{"alias": "Juan"}},
	})
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.TelegramIdentity).ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 when telegram is not linked, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTelegramIdentityGetPutFlow(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	t.Setenv("TELEGRAM_IDENTITY_V2_ENABLED", "true")
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	linkTelegramUserForTest(t, store, user.ID, 99887766, "expense_tester")

	getReq := httptest.NewRequest(http.MethodGet, "/api/telegram/identity", nil)
	getReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	getRec := httptest.NewRecorder()
	h.RequireAuth(h.TelegramIdentity).ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("initial GET status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	putReq := jsonRequest(t, http.MethodPut, "/api/telegram/identity", map[string]any{
		"aliases": []map[string]any{
			{"alias": "Joaquin Gonzalez", "confidence": 1},
		},
		"fingerprints": []map[string]any{
			{
				"bank_norm":     "Santander",
				"account_last4": "8536",
				"cbu_cvu_last4": "5368",
				"holder_norm":   "Joaquin Gonzalez",
				"confidence":    1,
			},
		},
	})
	putReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	putRec := httptest.NewRecorder()
	h.RequireAuth(h.TelegramIdentity).ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRec.Code, putRec.Body.String())
	}

	var payload telegramIdentityResponse
	if err := json.Unmarshal(putRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if !payload.Enabled || !payload.Premium || !payload.Linked {
		t.Fatalf("expected enabled/premium/linked true, got enabled=%v premium=%v linked=%v", payload.Enabled, payload.Premium, payload.Linked)
	}
	if len(payload.Aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(payload.Aliases))
	}
	if len(payload.Fingerprints) != 1 {
		t.Fatalf("expected 1 fingerprint, got %d", len(payload.Fingerprints))
	}

	aliases, err := store.GetTelegramIdentityAliases(user.ID)
	if err != nil {
		t.Fatalf("GetTelegramIdentityAliases: %v", err)
	}
	if len(aliases) != 1 || aliases[0].AliasNorm != "joaquin gonzalez" {
		t.Fatalf("unexpected alias persistence: %+v", aliases)
	}
	fps, err := store.GetTelegramOwnedAccountFingerprints(user.ID)
	if err != nil {
		t.Fatalf("GetTelegramOwnedAccountFingerprints: %v", err)
	}
	if len(fps) != 1 || fps[0].AccountLast4 != "8536" || fps[0].CBUCVULast4 != "5368" {
		t.Fatalf("unexpected fingerprint persistence: %+v", fps)
	}
}

func TestBotTelegramIdentityEndpoint(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, _ := createUserAndSession(t, store)
	t.Setenv("TELEGRAM_IDENTITY_V2_ENABLED", "true")
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	linkTelegramUserForTest(t, store, user.ID, 44556677, "bot_user")
	now := time.Now().UTC()
	if err := store.ReplaceTelegramIdentityAliases(user.ID, []storage.TelegramIdentityAlias{
		{AliasRaw: "Joaquin", Confidence: 1, Source: "test"},
	}, now); err != nil {
		t.Fatalf("seed aliases: %v", err)
	}

	req := jsonRequest(t, http.MethodPost, "/api/bot/telegram/identity", map[string]any{
		"telegram_user_id": 44556677,
	})
	rec := httptest.NewRecorder()
	h.GetBotTelegramIdentity(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bot identity status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload telegramIdentityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bot identity response: %v", err)
	}
	if !payload.Enabled || !payload.Linked {
		t.Fatalf("expected enabled=true and linked=true, got enabled=%v linked=%v", payload.Enabled, payload.Linked)
	}
	if len(payload.Aliases) != 1 {
		t.Fatalf("expected aliases for linked user, got %d", len(payload.Aliases))
	}
}
