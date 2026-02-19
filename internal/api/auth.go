package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	sessionCookieName       = "expense_session"
	sessionDuration         = 24 * time.Hour
	sessionRememberDuration = 7 * 24 * time.Hour
	minPasswordLength       = 8
	resetCodeTTL            = 15 * time.Minute
	verificationTokenTTL    = 24 * time.Hour
	maxLoginAttempts        = 3
	loginBlockDuration      = 10 * time.Minute
	authThrottleWindow      = 10 * time.Minute
	authThrottleBlock       = 15 * time.Minute
	maxRegisterAttempts     = 8
	maxResetReqAttempts     = 6
	maxResetConfAttempts    = 8
	resetCodeMaxAttempts    = 5
	userStatusActive        = "active"
	userStatusPending       = "pending"
)

type contextKey string

const userIDContextKey contextKey = "userID"

type authPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

type authUserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type authProfilePayload struct {
	Name string `json:"name"`
}

type loginAttempt struct {
	count       int
	lockedUntil time.Time
}

var loginAttemptMu = &sync.Mutex{}
var loginAttempts = map[string]*loginAttempt{}

type actionThrottle struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

var authThrottleMu = &sync.Mutex{}
var authThrottleState = map[string]*actionThrottle{}

type resetRequestPayload struct {
	Email string `json:"email"`
}

type resetConfirmPayload struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func userIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}

func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie == nil || cookie.Value == "" {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
			return
		}
		session, err := h.storage.GetSession(cookie.Value)
		if err != nil {
			if err == sql.ErrNoRows {
				clearSessionCookie(w, r)
				writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate session"})
			return
		}
		if time.Now().After(session.ExpiresAt) {
			_ = h.storage.DeleteSession(session.ID)
			clearSessionCookie(w, r)
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Session expired"})
			return
		}
		ctx := context.WithValue(r.Context(), userIDContextKey, session.UserID)
		next(w, r.WithContext(ctx))
	}
}

func (h *Handler) AuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	if blocked, _ := consumeAuthThrottle("register|"+readClientIP(r), maxRegisterAttempts); blocked {
		writeJSON(w, http.StatusTooManyRequests, ErrorResponse{Error: "Demasiadas solicitudes. Intenta de nuevo en unos minutos"})
		return
	}
	var payload authPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	email := normalizeEmail(payload.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid email"})
		return
	}
	if len(payload.Password) < minPasswordLength {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Password must be at least 8 characters"})
		return
	}
	if existing, err := h.storage.GetUserByEmail(email); err == nil {
		if strings.EqualFold(existing.Status, userStatusPending) {
			if err := h.issueEmailVerification(existing, r); err != nil {
				writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to send verification email"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "verification_sent"})
			return
		}
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Email already registered"})
		return
	} else if err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to check user"})
		return
	}
	hash, err := storage.HashPassword(payload.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to hash password"})
		return
	}
	user, err := h.storage.CreateUserWithStatus(email, hash, userStatusPending)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create user"})
		return
	}
	if err := h.issueEmailVerification(user, r); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to send verification email"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "verification_sent"})
}

func (h *Handler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	var payload authPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	email := normalizeEmail(payload.Email)
	key := loginKey(email, readClientIP(r))
	if blocked, _ := isLoginBlocked(key); blocked {
		writeJSON(w, http.StatusTooManyRequests, ErrorResponse{Error: "Demasiados intentos. Intenta de nuevo en unos minutos"})
		return
	}
	user, err := h.storage.GetUserByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			recordLoginFailure(key)
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to check user"})
		return
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		recordLoginFailure(key)
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
		return
	}
	if err := storage.ComparePassword(user.PasswordHash, payload.Password); err != nil {
		recordLoginFailure(key)
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
		return
	}
	clearLoginAttempts(key)
	if !strings.EqualFold(user.Status, userStatusActive) {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "Email no verificado. Revisa tu correo."})
		return
	}
	if err := h.createSession(w, r, user.ID, payload.Remember); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create session"})
		return
	}
	writeJSON(w, http.StatusOK, authUserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	})
}

func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie != nil && cookie.Value != "" {
		_ = h.storage.DeleteSession(cookie.Value)
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *Handler) AuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	user, err := h.storage.GetUserByID(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch user"})
		return
	}
	if !strings.EqualFold(user.Status, userStatusActive) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie != nil && cookie.Value != "" {
			_ = h.storage.DeleteSession(cookie.Value)
		}
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "Email no verificado"})
		return
	}
	writeJSON(w, http.StatusOK, authUserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	})
}

func (h *Handler) AuthUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var payload authProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	name, err := storage.ValidateUserName(payload.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Nombre invalido"})
		return
	}

	if err := h.storage.UpdateUserName(userID, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update profile"})
		return
	}

	user, err := h.storage.GetUserByID(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch user"})
		return
	}

	writeJSON(w, http.StatusOK, authUserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	})
}

func (h *Handler) AuthResetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	var payload resetRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	email := normalizeEmail(payload.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid email"})
		return
	}
	if blocked, _ := consumeAuthThrottle("reset-request|"+readClientIP(r)+"|"+email, maxResetReqAttempts); blocked {
		writeJSON(w, http.StatusTooManyRequests, ErrorResponse{Error: "Demasiadas solicitudes. Intenta de nuevo en unos minutos"})
		return
	}
	appBaseURL, err := externalBaseURL(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Configuracion incompleta del servidor"})
		return
	}
	user, err := h.storage.GetUserByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to process request"})
		return
	}
	code, err := newResetCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate code"})
		return
	}
	reset := storage.PasswordReset{
		UserID:      user.ID,
		CodeHash:    hashResetCode(code),
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(resetCodeTTL),
		MaxAttempts: resetCodeMaxAttempts,
	}
	if err := h.storage.CreatePasswordReset(reset); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create reset code"})
		return
	}
	if err := sendResetCodeEmail(email, code, appBaseURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to send reset code"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) AuthResetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	var payload resetConfirmPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	email := normalizeEmail(payload.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid email"})
		return
	}
	if blocked, _ := consumeAuthThrottle("reset-confirm|"+readClientIP(r)+"|"+email, maxResetConfAttempts); blocked {
		writeJSON(w, http.StatusTooManyRequests, ErrorResponse{Error: "Demasiados intentos. Solicita un nuevo codigo"})
		return
	}
	if len(payload.Password) < minPasswordLength {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Password must be at least 8 characters"})
		return
	}
	code := strings.TrimSpace(payload.Code)
	if len(code) < 4 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid code"})
		return
	}
	user, err := h.storage.GetUserByEmail(email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid code"})
		return
	}
	reset, err := h.storage.GetLatestPasswordReset(user.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid code"})
		return
	}
	if time.Now().After(reset.ExpiresAt) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Codigo expirado"})
		return
	}
	if hashResetCode(code) != reset.CodeHash {
		_, _, exhausted, failureErr := h.storage.RegisterPasswordResetFailure(reset.ID)
		if failureErr != nil && failureErr != sql.ErrNoRows {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo validar el codigo"})
			return
		}
		if exhausted {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Codigo invalido. Solicita uno nuevo"})
			return
		}
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Codigo invalido"})
		return
	}
	hash, err := storage.HashPassword(payload.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update password"})
		return
	}
	if err := h.storage.UpdateUserPassword(user.ID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update password"})
		return
	}
	_ = h.storage.MarkPasswordResetUsed(reset.ID)
	_ = h.storage.DeleteSessionsByUserID(user.ID)
	// Best-effort notification to improve account security visibility.
	if appBaseURL, baseErr := externalBaseURL(r); baseErr == nil {
		_ = sendPasswordChangedEmail(email, appBaseURL)
	}
	clearAuthThrottle("reset-confirm|" + readClientIP(r) + "|" + email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request, userID string, remember bool) error {
	sessionID, err := newSessionID()
	if err != nil {
		return err
	}
	duration := sessionDuration
	if remember {
		duration = sessionRememberDuration
	}
	now := time.Now()
	session := storage.Session{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
		IP:        readClientIP(r),
		UserAgent: r.UserAgent(),
	}
	if err := h.storage.CreateSession(session); err != nil {
		return err
	}
	setSessionCookie(w, r, session)
	return nil
}

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, session storage.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  session.ExpiresAt,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if trustProxyHeaders() {
		proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		if proto != "" {
			parts := strings.Split(proto, ",")
			return strings.EqualFold(strings.TrimSpace(parts[0]), "https")
		}
	}
	return false
}

func readClientIP(r *http.Request) string {
	if trustProxyHeaders() {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			for _, candidate := range strings.Split(xff, ",") {
				candidate = strings.TrimSpace(candidate)
				if net.ParseIP(candidate) != nil {
					return candidate
				}
			}
		}
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(xrip) != nil {
			return xrip
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if net.ParseIP(remote) != nil {
		return remote
	}
	return "unknown"
}

func loginKey(email, ip string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	ip = strings.TrimSpace(ip)
	return email + "|" + ip
}

func isLoginBlocked(key string) (bool, time.Duration) {
	loginAttemptMu.Lock()
	defer loginAttemptMu.Unlock()
	attempt, ok := loginAttempts[key]
	if !ok {
		return false, 0
	}
	if attempt.lockedUntil.IsZero() {
		return false, 0
	}
	if time.Now().After(attempt.lockedUntil) {
		delete(loginAttempts, key)
		return false, 0
	}
	return true, time.Until(attempt.lockedUntil)
}

func recordLoginFailure(key string) (bool, time.Duration) {
	loginAttemptMu.Lock()
	defer loginAttemptMu.Unlock()
	attempt, ok := loginAttempts[key]
	if !ok {
		attempt = &loginAttempt{}
		loginAttempts[key] = attempt
	}
	if !attempt.lockedUntil.IsZero() && time.Now().Before(attempt.lockedUntil) {
		return true, time.Until(attempt.lockedUntil)
	}
	attempt.count++
	if attempt.count >= maxLoginAttempts {
		attempt.lockedUntil = time.Now().Add(loginBlockDuration)
		return true, loginBlockDuration
	}
	return false, 0
}

func clearLoginAttempts(key string) {
	loginAttemptMu.Lock()
	defer loginAttemptMu.Unlock()
	delete(loginAttempts, key)
}

func consumeAuthThrottle(key string, maxAttempts int) (bool, time.Duration) {
	authThrottleMu.Lock()
	defer authThrottleMu.Unlock()

	now := time.Now()
	entry, ok := authThrottleState[key]
	if !ok {
		authThrottleState[key] = &actionThrottle{
			count:       1,
			windowStart: now,
		}
		return false, 0
	}
	if !entry.lockedUntil.IsZero() && now.Before(entry.lockedUntil) {
		return true, time.Until(entry.lockedUntil)
	}
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) > authThrottleWindow {
		entry.count = 1
		entry.windowStart = now
		entry.lockedUntil = time.Time{}
		return false, 0
	}

	entry.count++
	if entry.count > maxAttempts {
		entry.lockedUntil = now.Add(authThrottleBlock)
		return true, authThrottleBlock
	}
	return false, 0
}

func clearAuthThrottle(key string) {
	authThrottleMu.Lock()
	defer authThrottleMu.Unlock()
	delete(authThrottleState, key)
}

func trustProxyHeaders() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true")
}
