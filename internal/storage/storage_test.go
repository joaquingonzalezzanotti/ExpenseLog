package storage

import (
	"database/sql"
	"math"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func newPostgresStoreForTest(t *testing.T) Storage {
	t.Helper()

	uri := os.Getenv("TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration test")
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	host := parsed.Host
	if parsed.Path == "" || parsed.Path == "/" {
		t.Fatalf("TEST_DATABASE_URL missing database name")
	}
	dbName := parsed.Path[1:]

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

	baseConfig := SystemConfig{
		StorageURL:  host + "/" + dbName,
		StorageType: BackendTypePostgres,
		StorageUser: dbUser,
		StoragePass: pass,
		StorageSSL:  sslMode,
	}

	store, err := InitializePostgresStore(baseConfig)
	if err != nil {
		t.Fatalf("failed to init postgres store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createIntegrationUser(t *testing.T, store Storage) User {
	t.Helper()

	hash, err := HashPassword("testpassword")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	email := "test+" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	user, err := store.CreateUser(email, hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func addCAExpenseForTest(t *testing.T, store Storage, userID string, amount float64, when time.Time) {
	t.Helper()
	flow := "expense"
	if amount > 0 {
		flow = "income"
	}
	err := store.AddExpense(userID, Expense{
		Name:     "Seed CA",
		Category: "Ingresos",
		Amount:   amount,
		Currency: "ars",
		Source:   "CA",
		Flow:     flow,
		Date:     when,
	})
	if err != nil {
		t.Fatalf("seed expense: %v", err)
	}
}

func summarizeSignedBuckets(expenses []Expense) (total, income, expense float64) {
	for _, exp := range expenses {
		total += exp.Amount
		if exp.Amount > 0 {
			income += exp.Amount
		} else if exp.Amount < 0 {
			expense += math.Abs(exp.Amount)
		}
	}
	return total, income, expense
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestPostgresStoreCRUD(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	expense := Expense{
		Name:     "PG-Test",
		Category: "Test",
		Amount:   -50,
		Currency: "usd",
		Date:     time.Now(),
	}

	if err := store.AddExpense(user.ID, expense); err != nil {
		t.Fatalf("add expense: %v", err)
	}
	all, err := store.GetAllExpenses(user.ID)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(all) == 0 {
		t.Fatalf("expected expenses in postgres backend")
	}

	saved := all[0]
	saved.Amount = -75
	if err := store.UpdateExpense(user.ID, saved.ID, saved); err != nil {
		t.Fatalf("update expense: %v", err)
	}

	if err := store.RemoveExpense(user.ID, saved.ID); err != nil {
		t.Fatalf("remove expense: %v", err)
	}
}

func TestReconciliationRoundTrip_PositiveAdjustment(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	baseNow := time.Now().UTC().Truncate(time.Second)
	addCAExpenseForTest(t, store, user.ID, 100000, baseNow.Add(-2*time.Hour))

	applyResult, err := store.ApplyReconciliation(user.ID, ReconciliationApplyInput{
		TargetBalance: 105000,
		Currency:      "ars",
		Now:           baseNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if applyResult.Status != "applied" {
		t.Fatalf("expected status applied, got %s", applyResult.Status)
	}
	if !almostEqual(applyResult.Difference, 5000) {
		t.Fatalf("expected difference 5000, got %.2f", applyResult.Difference)
	}
	if applyResult.Expense.Flow != "income" || !almostEqual(applyResult.Expense.Amount, 5000) {
		t.Fatalf("unexpected adjustment expense: flow=%s amount=%.2f", applyResult.Expense.Flow, applyResult.Expense.Amount)
	}

	reversal, err := store.RevertReconciliation(user.ID, applyResult.Expense.ID, baseNow)
	if err != nil {
		t.Fatalf("revert reconciliation: %v", err)
	}
	if reversal.Flow != "expense" || !almostEqual(reversal.Amount, -5000) {
		t.Fatalf("unexpected reversal expense: flow=%s amount=%.2f", reversal.Flow, reversal.Amount)
	}

	all, err := store.GetAllExpenses(user.ID)
	if err != nil {
		t.Fatalf("get all expenses: %v", err)
	}
	total, income, expense := summarizeSignedBuckets(all)
	if !almostEqual(total, 100000) {
		t.Fatalf("expected total balance 100000 after round-trip, got %.2f", total)
	}
	// This documents the current dashboard-by-sign behavior:
	// positive adjustment increases ingresos and reversal appears as gasto.
	if !almostEqual(income, 105000) {
		t.Fatalf("expected gross income 105000, got %.2f", income)
	}
	if !almostEqual(expense, 5000) {
		t.Fatalf("expected gross expense 5000, got %.2f", expense)
	}
}

func TestReconciliationRoundTrip_NegativeAdjustment(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	baseNow := time.Now().UTC().Truncate(time.Second)
	addCAExpenseForTest(t, store, user.ID, 100000, baseNow.Add(-2*time.Hour))

	applyResult, err := store.ApplyReconciliation(user.ID, ReconciliationApplyInput{
		TargetBalance: 90000,
		Currency:      "ars",
		Now:           baseNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if applyResult.Status != "applied" {
		t.Fatalf("expected status applied, got %s", applyResult.Status)
	}
	if !almostEqual(applyResult.Difference, -10000) {
		t.Fatalf("expected difference -10000, got %.2f", applyResult.Difference)
	}
	if applyResult.Expense.Flow != "expense" || !almostEqual(applyResult.Expense.Amount, -10000) {
		t.Fatalf("unexpected adjustment expense: flow=%s amount=%.2f", applyResult.Expense.Flow, applyResult.Expense.Amount)
	}

	reversal, err := store.RevertReconciliation(user.ID, applyResult.Expense.ID, baseNow)
	if err != nil {
		t.Fatalf("revert reconciliation: %v", err)
	}
	if reversal.Flow != "income" || !almostEqual(reversal.Amount, 10000) {
		t.Fatalf("unexpected reversal expense: flow=%s amount=%.2f", reversal.Flow, reversal.Amount)
	}

	all, err := store.GetAllExpenses(user.ID)
	if err != nil {
		t.Fatalf("get all expenses: %v", err)
	}
	total, income, expense := summarizeSignedBuckets(all)
	if !almostEqual(total, 100000) {
		t.Fatalf("expected total balance 100000 after round-trip, got %.2f", total)
	}
	if !almostEqual(income, 110000) {
		t.Fatalf("expected gross income 110000, got %.2f", income)
	}
	if !almostEqual(expense, 10000) {
		t.Fatalf("expected gross expense 10000, got %.2f", expense)
	}
}

func TestRevertReconciliationTwiceReturnsConflict(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	baseNow := time.Now().UTC().Truncate(time.Second)
	addCAExpenseForTest(t, store, user.ID, 80000, baseNow.Add(-2*time.Hour))

	applyResult, err := store.ApplyReconciliation(user.ID, ReconciliationApplyInput{
		TargetBalance: 85000,
		Currency:      "ars",
		Now:           baseNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}

	if _, err := store.RevertReconciliation(user.ID, applyResult.Expense.ID, baseNow); err != nil {
		t.Fatalf("first revert should succeed, got: %v", err)
	}

	_, err = store.RevertReconciliation(user.ID, applyResult.Expense.ID, baseNow.Add(time.Minute))
	if err == nil {
		t.Fatalf("expected second revert to fail")
	}
	if err != ErrReconciliationAlreadyReverted {
		t.Fatalf("expected ErrReconciliationAlreadyReverted, got %v", err)
	}
}

func TestRevertReconciliationNotFound(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	_, err := store.RevertReconciliation(user.ID, "missing-adjustment-id", time.Now().UTC())
	if err == nil {
		t.Fatalf("expected not found error")
	}
	if err != ErrReconciliationNotFound {
		t.Fatalf("expected ErrReconciliationNotFound, got %v", err)
	}
}

func TestPasswordResetAttemptsAreLimitedAndInvalidateToken(t *testing.T) {
	store := newPostgresStoreForTest(t)
	user := createIntegrationUser(t, store)

	reset := PasswordReset{
		UserID:      user.ID,
		CodeHash:    "hash",
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
		MaxAttempts: 3,
	}
	if err := store.CreatePasswordReset(reset); err != nil {
		t.Fatalf("create reset: %v", err)
	}

	latest, err := store.GetLatestPasswordReset(user.ID)
	if err != nil {
		t.Fatalf("get latest reset: %v", err)
	}

	for i := 1; i <= 2; i++ {
		attempts, maxAttempts, exhausted, regErr := store.RegisterPasswordResetFailure(latest.ID)
		if regErr != nil {
			t.Fatalf("register failure #%d: %v", i, regErr)
		}
		if attempts != i {
			t.Fatalf("expected attempts=%d, got %d", i, attempts)
		}
		if maxAttempts != 3 {
			t.Fatalf("expected maxAttempts=3, got %d", maxAttempts)
		}
		if exhausted {
			t.Fatalf("did not expect reset to be exhausted at attempt %d", i)
		}
	}

	attempts, maxAttempts, exhausted, regErr := store.RegisterPasswordResetFailure(latest.ID)
	if regErr != nil {
		t.Fatalf("register failure #3: %v", regErr)
	}
	if attempts != 3 || maxAttempts != 3 || !exhausted {
		t.Fatalf("expected exhausted on third attempt, got attempts=%d max=%d exhausted=%v", attempts, maxAttempts, exhausted)
	}

	_, err = store.GetLatestPasswordReset(user.ID)
	if err == nil {
		t.Fatalf("expected no active reset after exhausting attempts")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows after exhaustion, got %v", err)
	}
}

func TestDeleteSessionsByUserIDRemovesAllUserSessions(t *testing.T) {
	store := newPostgresStoreForTest(t)
	userA := createIntegrationUser(t, store)
	userB := createIntegrationUser(t, store)

	now := time.Now().UTC()
	sessionA1 := Session{ID: "sess-a1-" + strconv.FormatInt(now.UnixNano(), 10), UserID: userA.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	sessionA2 := Session{ID: "sess-a2-" + strconv.FormatInt(now.UnixNano()+1, 10), UserID: userA.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	sessionB1 := Session{ID: "sess-b1-" + strconv.FormatInt(now.UnixNano()+2, 10), UserID: userB.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}

	if err := store.CreateSession(sessionA1); err != nil {
		t.Fatalf("create sessionA1: %v", err)
	}
	if err := store.CreateSession(sessionA2); err != nil {
		t.Fatalf("create sessionA2: %v", err)
	}
	if err := store.CreateSession(sessionB1); err != nil {
		t.Fatalf("create sessionB1: %v", err)
	}

	if err := store.DeleteSessionsByUserID(userA.ID); err != nil {
		t.Fatalf("delete sessions by user: %v", err)
	}

	if _, err := store.GetSession(sessionA1.ID); err != sql.ErrNoRows {
		t.Fatalf("expected sessionA1 deleted, got %v", err)
	}
	if _, err := store.GetSession(sessionA2.ID); err != sql.ErrNoRows {
		t.Fatalf("expected sessionA2 deleted, got %v", err)
	}
	if _, err := store.GetSession(sessionB1.ID); err != nil {
		t.Fatalf("expected sessionB1 to remain, got %v", err)
	}
}
