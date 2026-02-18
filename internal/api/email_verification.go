package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
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
	verifyURL := buildVerificationURL(r, token)
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

func buildVerificationURL(r *http.Request, token string) string {
	base := baseURLFromRequest(r)
	return fmt.Sprintf("%s/auth/verify?token=%s", base, url.QueryEscape(token))
}

func baseURLFromRequest(r *http.Request) string {
	if base := strings.TrimSpace(os.Getenv("APP_BASE_URL")); base != "" {
		return strings.TrimRight(base, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		parts := strings.Split(proto, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	host := strings.TrimSpace(r.Host)
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (h *Handler) AuthVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeVerificationPage(w, http.StatusMethodNotAllowed, "Metodo no permitido", "Este endpoint solo acepta GET.")
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeVerificationPage(w, http.StatusBadRequest, "Token invalido", "El token de verificacion es invalido o falta.")
		return
	}
	verification, err := h.storage.GetEmailVerificationByTokenHash(hashVerificationToken(token))
	if err != nil {
		writeVerificationPage(w, http.StatusBadRequest, "Token invalido", "El token de verificacion es invalido o expiro.")
		return
	}
	if !verification.VerifiedAt.IsZero() {
		writeVerificationPage(w, http.StatusOK, "Email ya verificado", "Tu email ya fue verificado.")
		return
	}
	if time.Now().After(verification.ExpiresAt) {
		writeVerificationPage(w, http.StatusBadRequest, "Token expirado", "El token expiro. Pedi uno nuevo.")
		return
	}
	if err := h.storage.UpdateUserStatus(verification.UserID, userStatusActive); err != nil {
		writeVerificationPage(w, http.StatusInternalServerError, "Error al verificar", "No pudimos verificar tu email. Intenta de nuevo.")
		return
	}
	if err := h.storage.MarkEmailVerificationUsed(verification.ID); err != nil {
		writeVerificationPage(w, http.StatusInternalServerError, "Error al verificar", "No pudimos completar la verificacion. Intenta de nuevo.")
		return
	}
	writeVerificationPage(w, http.StatusOK, "Email verificado", "Ya podes ingresar con tu cuenta.")
}

func writeVerificationPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    body { font-family: Arial, sans-serif; background: #0f1115; color: #e5e7eb; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
    .card { background: #151923; border: 1px solid #2b3245; border-radius: 12px; padding: 24px 28px; max-width: 520px; box-shadow: 0 12px 28px rgba(0,0,0,0.35); }
    h1 { font-size: 20px; margin: 0 0 10px; color: #f8fafc; }
    p { margin: 0 0 16px; color: #cbd5f5; }
    a { color: #7dd3fc; text-decoration: none; font-weight: 600; }
  </style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <p>%s</p>
    <a href="/app">Ir al login</a>
  </div>
</body>
</html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}

