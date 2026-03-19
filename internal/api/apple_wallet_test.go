package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func TestNormalizeWalletAmount(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{in: 12500, want: 12500},
		{in: 12500.125, want: 12500.13},
		{in: 0.004, want: 0},
		{in: 0.005, want: 0.01},
		{in: -10.235, want: -10.24},
	}
	for _, tc := range cases {
		got := normalizeWalletAmount(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeWalletAmount(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseWalletAmountString(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{in: "$ 20,00", want: 20, ok: true},
		{in: "ARS 1.234,56", want: 1234.56, ok: true},
		{in: "1,234.56", want: 1234.56, ok: true},
		{in: "15.5", want: 15.5, ok: true},
		{in: "", want: 0, ok: false},
	}
	for _, tc := range cases {
		got, ok := parseWalletAmountString(tc.in)
		if ok != tc.ok {
			t.Fatalf("parseWalletAmountString(%q) ok=%v, want %v", tc.in, ok, tc.ok)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("parseWalletAmountString(%q)=%v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMergeWalletPayloadFromRaw(t *testing.T) {
	payload := appleWalletIngestRequest{}
	raw := `{
		"merchantRaw":"Merpago*pachigonzalez",
		"shortcutInput":{"amount":"$ 20,00","paidAt":"2026-03-18T21:21:00-03:00"}
	}`
	mergeWalletPayloadFromRaw(&payload, raw)

	if payload.Amount != 20 {
		t.Fatalf("expected amount 20, got %.2f", payload.Amount)
	}
	if payload.MerchantRaw != "Merpago pachigonzalez" {
		t.Fatalf("expected merchantRaw populated, got %q", payload.MerchantRaw)
	}
	if payload.PaidAt.IsZero() {
		t.Fatalf("expected paidAt to be parsed")
	}
}

func newAPIStoreForTest(t *testing.T) storage.Storage {
	t.Helper()
	uri := os.Getenv("TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	dbUser := ""
	pass := ""
	if parsed.User != nil {
		dbUser = parsed.User.Username()
		pass, _ = parsed.User.Password()
	}
	sslMode := parsed.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}
	store, err := storage.InitializePostgresStore(storage.SystemConfig{
		StorageURL:  parsed.Host + parsed.Path,
		StorageType: storage.BackendTypePostgres,
		StorageUser: dbUser,
		StoragePass: pass,
		StorageSSL:  sslMode,
	})
	if err != nil {
		t.Fatalf("init postgres store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createUserAndSession(t *testing.T, s storage.Storage) (storage.User, string) {
	t.Helper()
	hash, _ := storage.HashPassword("testpassword")
	user, err := s.CreateUser("wallet+"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session := storage.Session{ID: "sess-" + strconv.FormatInt(time.Now().UnixNano(), 10), UserID: user.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour)}
	if err := s.CreateSession(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return user, session.ID
}

func setUserPlanTier(t *testing.T, userID, planTier string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(
		`INSERT INTO user_config (user_id, currency, start_date, plan_tier)
		 VALUES ($1, 'ars', 1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET plan_tier = EXCLUDED.plan_tier`,
		userID,
		storage.NormalizePlanTier(planTier),
	)
	if err != nil {
		t.Fatalf("set user plan tier: %v", err)
	}
}

func createWalletIngestToken(t *testing.T, h *Handler, sessionID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/token", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.CreateAppleWalletIngestToken).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected token create status: %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.Token == "" {
		t.Fatalf("token response is empty")
	}
	return body.Token
}

func TestAppleWalletDebug(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	_, sessionID := createUserAndSession(t, store)

	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/debug", bytes.NewBufferString(`{"sample":true}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.AppleWalletDebug).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppleWalletIngestCompleteAndDuplicate(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	token := createWalletIngestToken(t, h, sessionID)

	payload := `{"amount":12500,"merchant":"Starbucks","merchantRaw":"STARBUCKS STORE 2143","cardLabel":"Visa Galicia","walletCategory":"Food & Drink","paidAt":"2026-03-10T14:32:00-03:00","source":"apple_wallet_shortcut","rawPayload":{"shortcutInput":{"amount":12500,"merchant":"Starbucks"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.AppleWalletIngest(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected first ingest status: %d body=%s", rec.Code, rec.Body.String())
	}
	var firstBody appleWalletIngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("decode first ingest response: %v", err)
	}
	if firstBody.TransactionID == "" {
		t.Fatalf("expected transactionId in first ingest response")
	}
	firstExpense, err := store.GetExpense(user.ID, firstBody.TransactionID)
	if err != nil {
		t.Fatalf("load first created expense: %v", err)
	}
	if firstExpense.ReviewStatus != storage.ExpenseReviewStatusPending {
		t.Fatalf("expected review status %q, got %q", storage.ExpenseReviewStatusPending, firstExpense.ReviewStatus)
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	dupReq.Header.Set("Authorization", "Bearer "+token)
	dupRec := httptest.NewRecorder()
	h.AppleWalletIngest(dupRec, dupReq)
	if dupRec.Code != http.StatusOK {
		t.Fatalf("unexpected duplicate status: %d body=%s", dupRec.Code, dupRec.Body.String())
	}
	var dupBody map[string]string
	if err := json.Unmarshal(dupRec.Body.Bytes(), &dupBody); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if dupBody["status"] != walletEventStatusDuplicate {
		t.Fatalf("expected duplicate status, got %q", dupBody["status"])
	}
}

func TestAppleWalletIngestIncomplete(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	token := createWalletIngestToken(t, h, sessionID)

	payload := `{"merchant":"Unknown","source":"apple_wallet_shortcut","rawPayload":{"shortcutInput":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.AppleWalletIngest(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppleWalletIngestPremiumRequired(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)
	token := createWalletIngestToken(t, h, sessionID)
	setUserPlanTier(t, user.ID, storage.PlanTierFree)

	payload := `{"amount":12500,"merchant":"Starbucks","paidAt":"2026-03-10T14:32:00-03:00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.AppleWalletIngest(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppleWalletListEventsPremiumRequired(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierFree)

	req := httptest.NewRequest(http.MethodGet, "/api/integrations/apple-wallet/events?status=all&limit=10", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.ListAppleWalletEvents).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppleWalletListEventsByStatus(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)

	now := time.Now().UTC()
	if _, err := store.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:      user.ID,
		Source:      walletSourceDefault,
		Amount:      0,
		MerchantRaw: "SIN DATOS",
		PaidAt:      now,
		RawPayload:  `{"test":"needs_review"}`,
		Status:      walletEventStatusNeedsReview,
		Confidence:  walletConfidenceLow,
	}); err != nil {
		t.Fatalf("create needs_review event: %v", err)
	}
	if _, err := store.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:               user.ID,
		Source:               walletSourceDefault,
		Amount:               1000,
		Merchant:             "cafe",
		MerchantRaw:          "Cafe",
		PaidAt:               now.Add(-1 * time.Hour),
		RawPayload:           `{"test":"draft"}`,
		Status:               walletEventStatusDraftCreated,
		Confidence:           walletConfidenceMedium,
		CreatedTransactionID: "tx-demo-1",
	}); err != nil {
		t.Fatalf("create draft event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/integrations/apple-wallet/events?status=needs_review&limit=20", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.ListAppleWalletEvents).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var body appleWalletEventListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	if len(body.Items) == 0 {
		t.Fatalf("expected at least one event in list")
	}
	for _, item := range body.Items {
		if item.Status != walletEventStatusNeedsReview {
			t.Fatalf("expected only %q status, got %q", walletEventStatusNeedsReview, item.Status)
		}
	}
}

func TestAppleWalletResolveEventCreateDraft(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)

	event, err := store.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:      user.ID,
		Source:      walletSourceDefault,
		Amount:      0,
		MerchantRaw: "Cafe de prueba",
		PaidAt:      time.Now().UTC(),
		RawPayload:  `{"test":"resolve_create_draft"}`,
		Status:      walletEventStatusNeedsReview,
		Confidence:  walletConfidenceLow,
	})
	if err != nil {
		t.Fatalf("create wallet ingest event: %v", err)
	}

	reqBody := `{"eventId":"` + event.ID + `","action":"create_draft","amount":1234.5}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/events/resolve", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.ResolveAppleWalletEvent).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var body appleWalletResolveEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if body.TransactionID == "" {
		t.Fatalf("expected transactionId in response")
	}

	updated, err := store.GetWalletIngestEventByID(user.ID, event.ID)
	if err != nil {
		t.Fatalf("reload wallet event: %v", err)
	}
	if updated.Status != walletEventStatusDraftCreated {
		t.Fatalf("expected status %q, got %q", walletEventStatusDraftCreated, updated.Status)
	}
	if updated.CreatedTransactionID != body.TransactionID {
		t.Fatalf("expected created_transaction_id %q, got %q", body.TransactionID, updated.CreatedTransactionID)
	}
	createdExpense, err := store.GetExpense(user.ID, body.TransactionID)
	if err != nil {
		t.Fatalf("load created expense: %v", err)
	}
	if createdExpense.ReviewStatus != storage.ExpenseReviewStatusPending {
		t.Fatalf("expected review status %q, got %q", storage.ExpenseReviewStatusPending, createdExpense.ReviewStatus)
	}
}

func TestAppleWalletResolveEventRejectConflict(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)

	event, err := store.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:               user.ID,
		Source:               walletSourceDefault,
		Amount:               8900,
		Merchant:             "resto",
		MerchantRaw:          "Resto",
		PaidAt:               time.Now().UTC(),
		RawPayload:           `{"test":"reject_conflict"}`,
		Status:               walletEventStatusDraftCreated,
		Confidence:           walletConfidenceHigh,
		CreatedTransactionID: "tx-existing-1",
	})
	if err != nil {
		t.Fatalf("create wallet ingest event: %v", err)
	}

	reqBody := `{"eventId":"` + event.ID + `","action":"reject"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/events/resolve", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	h.RequireAuth(h.ResolveAppleWalletEvent).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}
