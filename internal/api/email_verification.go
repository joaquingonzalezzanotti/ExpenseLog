package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func (h *Handler) issueEmailVerification(user storage.User, r *http.Request) error {
	token, err := newVerificationToken()
	if err != nil {
		return err
	}
	verification := storage.EmailVerification{
		UserID:    user.ID,
		TokenHash: hashVerificationToken(token),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(verificationTokenTTL),
	}
	if err := h.storage.CreateEmailVerification(verification); err != nil {
		return err
	}
	verifyURL, err := buildVerificationURL(r, token)
	if err != nil {
		return err
	}
	return sendVerificationEmail(user.Email, verifyURL)
}

func newVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashVerificationToken(token string) string {
	normalized := strings.TrimSpace(token)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func buildVerificationURL(r *http.Request, token string) (string, error) {
	base, err := externalBaseURL(r)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/api/auth/verify?token=%s", base, url.QueryEscape(token)), nil
}

func baseURLFromRequest(r *http.Request) string {
	if base, ok := configuredAppBaseURL(); ok {
		return base
	}
	return requestBaseURL(r)
}

func configuredAppBaseURL() (string, bool) {
	base := strings.TrimSpace(os.Getenv("APP_BASE_URL"))
	if base == "" {
		return "", false
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), true
}

func requestBaseURL(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.ContainsAny(host, " /\\") {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if trustProxyHeaders() {
		proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		parts := strings.Split(proto, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func externalBaseURL(r *http.Request) (string, error) {
	if base, ok := configuredAppBaseURL(); ok {
		return base, nil
	}
	if isLocalHost(r.Host) {
		if base := requestBaseURL(r); base != "" {
			return base, nil
		}
	}
	return "", fmt.Errorf("APP_BASE_URL is required")
}

func isLocalHost(host string) bool {
	h := strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(h); err == nil {
		h = parsedHost
	}
	h = strings.Trim(h, "[]")
	switch strings.ToLower(h) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func (h *Handler) AuthVerifyEmail(w http.ResponseWriter, r *http.Request) {
	loginURL := "/app"
	if base, err := externalBaseURL(r); err == nil {
		loginURL = base + "/app"
	} else if base := baseURLFromRequest(r); base != "" {
		loginURL = base + "/app"
	}
	if r.Method != http.MethodGet {
		writeVerificationPage(w, http.StatusMethodNotAllowed, "Metodo no permitido", "Este endpoint solo acepta GET.", loginURL, false)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeVerificationPage(w, http.StatusBadRequest, "Token invalido", "El token de verificacion es invalido o falta.", loginURL, false)
		return
	}
	verification, err := h.storage.GetEmailVerificationByTokenHash(hashVerificationToken(token))
	if err != nil {
		writeVerificationPage(w, http.StatusBadRequest, "Token invalido", "El token de verificacion es invalido o expiro.", loginURL, false)
		return
	}
	if !verification.VerifiedAt.IsZero() {
		writeVerificationPage(w, http.StatusOK, "Email ya verificado", "Tu email ya fue verificado.", loginURL, true)
		return
	}
	if time.Now().After(verification.ExpiresAt) {
		writeVerificationPage(w, http.StatusBadRequest, "Token expirado", "El token expiro. Pedi uno nuevo.", loginURL, false)
		return
	}
	if err := h.storage.UpdateUserStatus(verification.UserID, userStatusActive); err != nil {
		writeVerificationPage(w, http.StatusInternalServerError, "Error al verificar", "No pudimos verificar tu email. Intenta de nuevo.", loginURL, false)
		return
	}
	if err := h.storage.MarkEmailVerificationUsed(verification.ID); err != nil {
		writeVerificationPage(w, http.StatusInternalServerError, "Error al verificar", "No pudimos completar la verificacion. Intenta de nuevo.", loginURL, false)
		return
	}
	writeVerificationPage(w, http.StatusOK, "Email verificado", "Ya podes ingresar con tu cuenta.", loginURL, true)
}

func writeVerificationPage(w http.ResponseWriter, status int, title, message, loginURL string, autoRedirect bool) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(status)
	redirectMeta := ""
	redirectHint := ""
	if autoRedirect {
		redirectMeta = fmt.Sprintf(`<meta http-equiv="refresh" content="2;url=%s">`, html.EscapeString(loginURL))
		redirectHint = `<p class="hint">Redirigiendo al login en 2 segundos...</p>`
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  %s
  <title>%s</title>
  <style>
    body {
      font-family: Arial, sans-serif;
      background:
        radial-gradient(circle at top left, rgba(37,99,235,0.18), transparent 40%%),
        #0a0f1d;
      color: #e5e7eb;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      margin: 0;
      padding: 1rem;
      box-sizing: border-box;
    }
    .card {
      background: #11192d;
      border: 1px solid #2a3b63;
      border-radius: 14px;
      padding: 26px 28px;
      max-width: 520px;
      width: 100%%;
      box-shadow: 0 18px 32px rgba(0,0,0,0.45);
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 0.4rem;
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.02em;
      color: #93c5fd;
      background: rgba(37,99,235,0.16);
      border: 1px solid rgba(96,165,250,0.4);
      border-radius: 999px;
      padding: 0.3rem 0.55rem;
      margin-bottom: 0.75rem;
    }
    h1 { font-size: 24px; margin: 0 0 10px; color: #f8fafc; }
    p { margin: 0 0 14px; color: #cbd5f5; font-size: 17px; line-height: 1.45; }
    .hint { margin: 0 0 18px; color: #93c5fd; font-size: 14px; }
    .cta {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      padding: 0.72rem 1rem;
      border-radius: 10px;
      background: #2563eb;
      color: #ffffff;
      text-decoration: none;
      font-weight: 700;
    }
    .cta:hover { background: #1d4ed8; }
  </style>
</head>
<body>
  <div class="card">
    <span class="badge">ExpenseLog</span>
    <h1>%s</h1>
    <p>%s</p>
    %s
    <a class="cta" href="%s">Ir al login</a>
  </div>
</body>
</html>`, redirectMeta, html.EscapeString(title), html.EscapeString(title), html.EscapeString(message), redirectHint, html.EscapeString(loginURL))
}
