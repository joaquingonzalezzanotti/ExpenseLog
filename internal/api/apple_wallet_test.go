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
