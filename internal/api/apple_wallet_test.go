package api

import (
	"bytes"
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
	user, _ := createUserAndSession(t, store)
	t.Setenv("EXPENSELOG_SHORTCUT_INGEST_TOKENS", user.ID+":secret123")

	payload := `{"amount":12500,"merchant":"Starbucks","merchantRaw":"STARBUCKS STORE 2143","cardLabel":"Visa Galicia","walletCategory":"Food & Drink","paidAt":"2026-03-10T14:32:00-03:00","source":"apple_wallet_shortcut","rawPayload":{"shortcutInput":{"amount":12500,"merchant":"Starbucks"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer secret123")
	rec := httptest.NewRecorder()
	h.AppleWalletIngest(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected first ingest status: %d body=%s", rec.Code, rec.Body.String())
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	dupReq.Header.Set("Authorization", "Bearer secret123")
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
	user, _ := createUserAndSession(t, store)
	t.Setenv("EXPENSELOG_SHORTCUT_INGEST_TOKENS", user.ID+":secret123")

	payload := `{"merchant":"Unknown","source":"apple_wallet_shortcut","rawPayload":{"shortcutInput":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/apple-wallet/ingest", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer secret123")
	rec := httptest.NewRecorder()
	h.AppleWalletIngest(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}
