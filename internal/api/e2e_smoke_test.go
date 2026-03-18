package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func jsonRequest(t *testing.T, method, target string, payload any) *http.Request {
	t.Helper()
	var body *bytes.Buffer
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewBuffer(raw)
	} else {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:4321"
	return req
}

func TestE2ESmokeAuthLifecycle(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	t.Setenv("APP_BASE_URL", "http://localhost:8080")

	var capturedVerifyURL string
	var capturedResetCode string
	var passwordChangedNotifications int

	origVerificationSender := sendVerificationEmailFn
	origResetSender := sendResetCodeEmailFn
	origPasswordChangedSender := sendPasswordChangedEmailFn
	sendVerificationEmailFn = func(toEmail, verifyURL string) error {
		capturedVerifyURL = verifyURL
		return nil
	}
	sendResetCodeEmailFn = func(toEmail, code, appURL string) error {
		capturedResetCode = strings.TrimSpace(code)
		return nil
	}
	sendPasswordChangedEmailFn = func(toEmail, appURL string) error {
		passwordChangedNotifications++
		return nil
	}
	t.Cleanup(func() {
		sendVerificationEmailFn = origVerificationSender
		sendResetCodeEmailFn = origResetSender
		sendPasswordChangedEmailFn = origPasswordChangedSender
	})

	email := "smoke-auth+" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	registerReq := jsonRequest(t, http.MethodPost, "/api/auth/register", map[string]any{
		"email":    email,
		"password": "Password123!",
	})
	registerRec := httptest.NewRecorder()
	h.AuthRegister(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registerRec.Code, registerRec.Body.String())
	}
	if capturedVerifyURL == "" {
		t.Fatalf("expected verification URL to be captured")
	}

	parsedVerifyURL, err := url.Parse(capturedVerifyURL)
	if err != nil {
		t.Fatalf("parse verify url: %v", err)
	}
	token := strings.TrimSpace(parsedVerifyURL.Query().Get("token"))
	if token == "" {
		t.Fatalf("verification token missing in captured URL")
	}

	verifyReq := httptest.NewRequest(http.MethodGet, "/api/auth/verify?token="+url.QueryEscape(token), nil)
	verifyReq.Host = "localhost:8080"
	verifyRec := httptest.NewRecorder()
	h.AuthVerifyEmail(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", verifyRec.Code, verifyRec.Body.String())
	}

	loginReq := jsonRequest(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    email,
		"password": "Password123!",
	})
	loginRec := httptest.NewRecorder()
	h.AuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	resetReq := jsonRequest(t, http.MethodPost, "/api/auth/reset/request", map[string]any{
		"email": email,
	})
	resetRec := httptest.NewRecorder()
	h.AuthResetRequest(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("reset request status=%d body=%s", resetRec.Code, resetRec.Body.String())
	}
	if capturedResetCode == "" {
		t.Fatalf("expected reset code to be captured")
	}

	resetConfirmReq := jsonRequest(t, http.MethodPost, "/api/auth/reset/confirm", map[string]any{
		"email":    email,
		"code":     capturedResetCode,
		"password": "NewPassword123!",
	})
	resetConfirmRec := httptest.NewRecorder()
	h.AuthResetConfirm(resetConfirmRec, resetConfirmReq)
	if resetConfirmRec.Code != http.StatusOK {
		t.Fatalf("reset confirm status=%d body=%s", resetConfirmRec.Code, resetConfirmRec.Body.String())
	}
	if passwordChangedNotifications == 0 {
		t.Fatalf("expected password changed notification to be sent")
	}

	oldPasswordLoginReq := jsonRequest(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    email,
		"password": "Password123!",
	})
	oldPasswordLoginRec := httptest.NewRecorder()
	h.AuthLogin(oldPasswordLoginRec, oldPasswordLoginReq)
	if oldPasswordLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("old password should fail, got status=%d body=%s", oldPasswordLoginRec.Code, oldPasswordLoginRec.Body.String())
	}

	newPasswordLoginReq := jsonRequest(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    email,
		"password": "NewPassword123!",
	})
	newPasswordLoginRec := httptest.NewRecorder()
	h.AuthLogin(newPasswordLoginRec, newPasswordLoginReq)
	if newPasswordLoginRec.Code != http.StatusOK {
		t.Fatalf("new password login status=%d body=%s", newPasswordLoginRec.Code, newPasswordLoginRec.Body.String())
	}
}

func TestE2ESmokeExpenseRecurringAndExport(t *testing.T) {
	store := newAPIStoreForTest(t)
	h := NewHandler(store)
	user, sessionID := createUserAndSession(t, store)
	setUserPlanTier(t, user.ID, storage.PlanTierPremium)

	addExpenseReq := jsonRequest(t, http.MethodPut, "/api/expense", map[string]any{
		"name":     "Compra smoke",
		"category": "Comida",
		"amount":   1500,
		"currency": "ars",
		"flow":     "expense",
		"date":     time.Now().Format(time.RFC3339),
	})
	addExpenseReq.Header.Set("Idempotency-Key", "smoke-expense-add")
	addExpenseReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	addExpenseRec := httptest.NewRecorder()
	h.RequireAuth(h.AddExpense).ServeHTTP(addExpenseRec, addExpenseReq)
	if addExpenseRec.Code != http.StatusOK {
		t.Fatalf("add expense status=%d body=%s", addExpenseRec.Code, addExpenseRec.Body.String())
	}

	addExpenseDupReq := jsonRequest(t, http.MethodPut, "/api/expense", map[string]any{
		"name":     "Compra smoke",
		"category": "Comida",
		"amount":   1500,
		"currency": "ars",
		"flow":     "expense",
		"date":     time.Now().Format(time.RFC3339),
	})
	addExpenseDupReq.Header.Set("Idempotency-Key", "smoke-expense-add")
	addExpenseDupReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	addExpenseDupRec := httptest.NewRecorder()
	h.RequireAuth(h.AddExpense).ServeHTTP(addExpenseDupRec, addExpenseDupReq)
	if addExpenseDupRec.Code != http.StatusConflict {
		t.Fatalf("duplicate add expense status=%d body=%s", addExpenseDupRec.Code, addExpenseDupRec.Body.String())
	}

	getExpensesReq := httptest.NewRequest(http.MethodGet, "/api/expenses", nil)
	getExpensesReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	getExpensesRec := httptest.NewRecorder()
	h.RequireAuth(h.GetExpenses).ServeHTTP(getExpensesRec, getExpensesReq)
	if getExpensesRec.Code != http.StatusOK {
		t.Fatalf("get expenses status=%d body=%s", getExpensesRec.Code, getExpensesRec.Body.String())
	}
	var expenses []storage.Expense
	if err := json.Unmarshal(getExpensesRec.Body.Bytes(), &expenses); err != nil {
		t.Fatalf("decode expenses: %v", err)
	}
	if len(expenses) == 0 {
		t.Fatalf("expected at least one expense")
	}
	expenseID := expenses[0].ID

	editExpenseReq := jsonRequest(t, http.MethodPut, "/api/expense/edit?id="+url.QueryEscape(expenseID), map[string]any{
		"name":     "Compra smoke editada",
		"category": "Comida",
		"amount":   2000,
		"currency": "ars",
		"flow":     "expense",
		"date":     time.Now().Format(time.RFC3339),
	})
	editExpenseReq.Header.Set("Idempotency-Key", "smoke-expense-edit")
	editExpenseReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	editExpenseRec := httptest.NewRecorder()
	h.RequireAuth(h.EditExpense).ServeHTTP(editExpenseRec, editExpenseReq)
	if editExpenseRec.Code != http.StatusOK {
		t.Fatalf("edit expense status=%d body=%s", editExpenseRec.Code, editExpenseRec.Body.String())
	}

	deleteExpenseReq := httptest.NewRequest(http.MethodDelete, "/api/expense/delete?id="+url.QueryEscape(expenseID), nil)
	deleteExpenseReq.Header.Set("Idempotency-Key", "smoke-expense-delete")
	deleteExpenseReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	deleteExpenseRec := httptest.NewRecorder()
	h.RequireAuth(h.DeleteExpense).ServeHTTP(deleteExpenseRec, deleteExpenseReq)
	if deleteExpenseRec.Code != http.StatusOK {
		t.Fatalf("delete expense status=%d body=%s", deleteExpenseRec.Code, deleteExpenseRec.Body.String())
	}

	recurringPayload := map[string]any{
		"name":        "Servicio smoke",
		"amount":      999,
		"category":    "Servicios",
		"tags":        []string{"smoke"},
		"interval":    "monthly",
		"startDate":   time.Now().Format(time.RFC3339),
		"occurrences": 2,
		"flow":        "expense",
	}
	addRecurringReq := jsonRequest(t, http.MethodPut, "/api/recurring-expense", recurringPayload)
	addRecurringReq.Header.Set("Idempotency-Key", "smoke-recurring-add")
	addRecurringReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	addRecurringRec := httptest.NewRecorder()
	h.RequireAuth(h.AddRecurringExpense).ServeHTTP(addRecurringRec, addRecurringReq)
	if addRecurringRec.Code != http.StatusCreated {
		t.Fatalf("add recurring status=%d body=%s", addRecurringRec.Code, addRecurringRec.Body.String())
	}

	getRecurringReq := httptest.NewRequest(http.MethodGet, "/api/recurring-expenses", nil)
	getRecurringReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	getRecurringRec := httptest.NewRecorder()
	h.RequireAuth(h.GetRecurringExpenses).ServeHTTP(getRecurringRec, getRecurringReq)
	if getRecurringRec.Code != http.StatusOK {
		t.Fatalf("get recurring status=%d body=%s", getRecurringRec.Code, getRecurringRec.Body.String())
	}
	var recurringItems []storage.RecurringExpense
	if err := json.Unmarshal(getRecurringRec.Body.Bytes(), &recurringItems); err != nil {
		t.Fatalf("decode recurring items: %v", err)
	}
	if len(recurringItems) == 0 {
		t.Fatalf("expected recurring item after create")
	}
	recurringID := recurringItems[0].ID

	updateRecurringReq := jsonRequest(t, http.MethodPut, "/api/recurring-expense/edit?id="+url.QueryEscape(recurringID)+"&updateAll=false", map[string]any{
		"name":        "Servicio smoke editado",
		"amount":      1200,
		"category":    "Servicios",
		"tags":        []string{"smoke"},
		"interval":    "monthly",
		"startDate":   time.Now().Format(time.RFC3339),
		"occurrences": 2,
		"flow":        "expense",
	})
	updateRecurringReq.Header.Set("Idempotency-Key", "smoke-recurring-update")
	updateRecurringReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	updateRecurringRec := httptest.NewRecorder()
	h.RequireAuth(h.UpdateRecurringExpense).ServeHTTP(updateRecurringRec, updateRecurringReq)
	if updateRecurringRec.Code != http.StatusOK {
		t.Fatalf("update recurring status=%d body=%s", updateRecurringRec.Code, updateRecurringRec.Body.String())
	}

	deleteRecurringReq := httptest.NewRequest(http.MethodDelete, "/api/recurring-expense/delete?id="+url.QueryEscape(recurringID)+"&removeAll=true", nil)
	deleteRecurringReq.Header.Set("Idempotency-Key", "smoke-recurring-delete")
	deleteRecurringReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	deleteRecurringRec := httptest.NewRecorder()
	h.RequireAuth(h.DeleteRecurringExpense).ServeHTTP(deleteRecurringRec, deleteRecurringReq)
	if deleteRecurringRec.Code != http.StatusOK {
		t.Fatalf("delete recurring status=%d body=%s", deleteRecurringRec.Code, deleteRecurringRec.Body.String())
	}

	now := time.Now()
	xlsxReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/export/monthly/xlsx?year=%d&month=%d&currency=ars", now.Year(), int(now.Month())), nil)
	xlsxReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	xlsxRec := httptest.NewRecorder()
	h.RequireAuth(h.ExportMonthlyXLSX).ServeHTTP(xlsxRec, xlsxReq)
	if xlsxRec.Code != http.StatusOK {
		t.Fatalf("export xlsx status=%d body=%s", xlsxRec.Code, xlsxRec.Body.String())
	}

	pdfReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/export/monthly/pdf?year=%d&month=%d&currency=ars", now.Year(), int(now.Month())), nil)
	pdfReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	pdfRec := httptest.NewRecorder()
	h.RequireAuth(h.ExportMonthlyPDF).ServeHTTP(pdfRec, pdfReq)
	if pdfRec.Code != http.StatusOK {
		t.Fatalf("export pdf status=%d body=%s", pdfRec.Code, pdfRec.Body.String())
	}
}
