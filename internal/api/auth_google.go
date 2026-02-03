package api

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/tanq16/expenseowl/internal/storage"
)

const (
	googleProviderName         = "google"
	googleOAuthAuthURL         = "https://accounts.google.com/o/oauth2/v2/auth"
	googleOAuthTokenURL        = "https://oauth2.googleapis.com/token"
	googleOAuthUserInfoURL     = "https://openidconnect.googleapis.com/v1/userinfo"
	googleOAuthScope           = "openid email profile"
	googleOAuthStateCookieName = "expense_google_oauth_state"
	googleOAuthStateTTL        = 10 * time.Minute
	googleOAuthSessionRemember = true
)

var googleOAuthHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

type googleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

type googleUserInfoResponse struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func (h *Handler) AuthGoogleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	cfg, err := loadGoogleOAuthConfig(r)
	if err != nil {
		redirectAuthError(w, r, "Google login no esta configurado")
		return
	}
	state, err := newGoogleOAuthState()
	if err != nil {
		redirectAuthError(w, r, "No se pudo iniciar login con Google")
		return
	}
	setGoogleOAuthStateCookie(w, r, state)
	http.Redirect(w, r, buildGoogleOAuthURL(cfg, state), http.StatusFound)
}

func (h *Handler) AuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	cfg, err := loadGoogleOAuthConfig(r)
	if err != nil {
		redirectAuthError(w, r, "Google login no esta configurado")
		return
	}
	if oauthError := strings.TrimSpace(r.URL.Query().Get("error")); oauthError != "" {
		clearGoogleOAuthStateCookie(w, r)
		redirectAuthError(w, r, googleOAuthErrorMessage(oauthError))
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" || !isValidGoogleOAuthState(r, state) {
		clearGoogleOAuthStateCookie(w, r)
		redirectAuthError(w, r, "No pudimos validar el login con Google")
		return
	}
	clearGoogleOAuthStateCookie(w, r)

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		redirectAuthError(w, r, "Google no devolvio un codigo valido")
		return
	}

	token, err := exchangeGoogleAuthCode(cfg, code)
	if err != nil {
		redirectAuthError(w, r, "No pudimos validar tu cuenta de Google")
		return
	}
	userInfo, err := fetchGoogleUserInfo(token.AccessToken)
	if err != nil {
		redirectAuthError(w, r, "No pudimos leer los datos de tu cuenta Google")
		return
	}
	if strings.TrimSpace(userInfo.Sub) == "" || normalizeEmail(userInfo.Email) == "" {
		redirectAuthError(w, r, "Google no devolvio un email valido")
		return
	}
	if !userInfo.EmailVerified {
		redirectAuthError(w, r, "Tu email de Google no esta verificado")
		return
	}

	user, err := h.resolveGoogleUser(userInfo)
	if err != nil {
		redirectAuthError(w, r, "No pudimos completar el login con Google")
		return
	}
	if err := h.createSession(w, r, user.ID, googleOAuthSessionRemember); err != nil {
		redirectAuthError(w, r, "No pudimos crear la sesion")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) resolveGoogleUser(info googleUserInfoResponse) (storage.User, error) {
	providerUserID := strings.TrimSpace(info.Sub)
	email := normalizeEmail(info.Email)

	identity, err := h.storage.GetOAuthIdentity(googleProviderName, providerUserID)
	if err == nil {
		user, getUserErr := h.storage.GetUserByID(identity.UserID)
		if getUserErr == nil {
			if strings.EqualFold(user.Status, userStatusPending) {
				if err := h.storage.UpdateUserStatus(user.ID, userStatusActive); err != nil {
					return storage.User{}, err
				}
				user.Status = userStatusActive
			}
			return user, nil
		}
		if getUserErr != sql.ErrNoRows {
			return storage.User{}, getUserErr
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return storage.User{}, err
	}

	user, err := h.storage.GetUserByEmail(email)
	if err == sql.ErrNoRows {
		created, createErr := h.storage.CreateUserWithStatus(email, "", userStatusActive)
		if createErr != nil {
			return storage.User{}, createErr
		}
		if identityErr := h.ensureGoogleIdentity(created.ID, email, providerUserID); identityErr != nil {
			return storage.User{}, identityErr
		}
		return created, nil
	}
	if err != nil {
		return storage.User{}, err
	}
	if strings.EqualFold(user.Status, userStatusPending) {
		if err := h.storage.UpdateUserStatus(user.ID, userStatusActive); err != nil {
			return storage.User{}, err
		}
		user.Status = userStatusActive
	}
	if err := h.ensureGoogleIdentity(user.ID, email, providerUserID); err != nil {
		return storage.User{}, err
	}
	return user, nil
}

func (h *Handler) ensureGoogleIdentity(userID, email, providerUserID string) error {
	err := h.storage.CreateOAuthIdentity(storage.OAuthIdentity{
		UserID:         userID,
		Provider:       googleProviderName,
		ProviderUserID: providerUserID,
		Email:          email,
	})
	if err == nil {
		return nil
	}

	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || string(pqErr.Code) != "23505" {
		return err
	}
	existing, getErr := h.storage.GetOAuthIdentity(googleProviderName, providerUserID)
	if getErr != nil {
		return err
	}
	if existing.UserID != userID {
		return fmt.Errorf("google identity linked to a different user")
	}
	return nil
}

func loadGoogleOAuthConfig(r *http.Request) (googleOAuthConfig, error) {
	cfg := googleOAuthConfig{
		ClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		RedirectURL:  strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL")),
	}
	if cfg.RedirectURL == "" {
		cfg.RedirectURL = strings.TrimRight(baseURLFromRequest(r), "/") + "/auth/google/callback"
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return googleOAuthConfig{}, fmt.Errorf("missing google oauth config")
	}
	return cfg, nil
}

func newGoogleOAuthState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func buildGoogleOAuthURL(cfg googleOAuthConfig, state string) string {
	params := url.Values{}
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", cfg.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", googleOAuthScope)
	params.Set("state", state)
	params.Set("access_type", "online")
	params.Set("prompt", "select_account")
	return googleOAuthAuthURL + "?" + params.Encode()
}

func setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookieName,
		Value:    state,
		Path:     "/auth/google/callback",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Now().Add(googleOAuthStateTTL),
	})
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookieName,
		Value:    "",
		Path:     "/auth/google/callback",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func isValidGoogleOAuthState(r *http.Request, expected string) bool {
	cookie, err := r.Cookie(googleOAuthStateCookieName)
	if err != nil || cookie == nil || cookie.Value == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expected)) == 1
}

func exchangeGoogleAuthCode(cfg googleOAuthConfig, code string) (googleTokenResponse, error) {
	payload := url.Values{}
	payload.Set("code", code)
	payload.Set("client_id", cfg.ClientID)
	payload.Set("client_secret", cfg.ClientSecret)
	payload.Set("redirect_uri", cfg.RedirectURL)
	payload.Set("grant_type", "authorization_code")

	req, err := http.NewRequest(http.MethodPost, googleOAuthTokenURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return googleTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := googleOAuthHTTPClient.Do(req)
	if err != nil {
		return googleTokenResponse{}, err
	}
	defer resp.Body.Close()

	var token googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return googleTokenResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return googleTokenResponse{}, fmt.Errorf("google token exchange failed: %s", token.Error)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return googleTokenResponse{}, fmt.Errorf("google token response missing access_token")
	}
	return token, nil
}

func fetchGoogleUserInfo(accessToken string) (googleUserInfoResponse, error) {
	req, err := http.NewRequest(http.MethodGet, googleOAuthUserInfoURL, nil)
	if err != nil {
		return googleUserInfoResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := googleOAuthHTTPClient.Do(req)
	if err != nil {
		return googleUserInfoResponse{}, err
	}
	defer resp.Body.Close()

	var userInfo googleUserInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return googleUserInfoResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return googleUserInfoResponse{}, fmt.Errorf("google user info request failed")
	}
	return userInfo, nil
}

func googleOAuthErrorMessage(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "access_denied":
		return "Cancelaste el acceso con Google"
	default:
		return "No se pudo completar el login con Google"
	}
}

func redirectAuthError(w http.ResponseWriter, r *http.Request, message string) {
	target := "/?auth_error=" + url.QueryEscape(strings.TrimSpace(message))
	http.Redirect(w, r, target, http.StatusFound)
}
