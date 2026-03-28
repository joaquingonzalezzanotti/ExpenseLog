package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	whatsAppLinkCodeTTL   = 10 * time.Minute
	whatsAppCodeAlphabet  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	whatsAppCodeLength    = 8
	whatsAppLinkCmdPrefix = "/vincular"
)

type whatsAppLinkStatusResponse struct {
	Provider      string                  `json:"provider"`
	Premium       bool                    `json:"premium"`
	Available     bool                    `json:"available"`
	Linked        bool                    `json:"linked"`
	WhatsAppPhone string                  `json:"whatsapp_phone,omitempty"`
	LinkedAt      *time.Time              `json:"linked_at,omitempty"`
	ActiveCode    *whatsAppActiveCodeView `json:"active_code,omitempty"`
	Number        string                  `json:"number,omitempty"`
	NumberDisplay string                  `json:"number_display,omitempty"`
	ChatURL       string                  `json:"chat_url,omitempty"`
}

type whatsAppActiveCodeView struct {
	CodeMasked string    `json:"code_masked"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type whatsAppCreateCodeResponse struct {
	Code          string    `json:"code"`
	ExpiresAt     time.Time `json:"expires_at"`
	ChatURL       string    `json:"chat_url,omitempty"`
	DeepLinkURL   string    `json:"deep_link_url,omitempty"`
	NumberDisplay string    `json:"number_display,omitempty"`
}

func sanitizeWhatsAppNumber(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	number := b.String()
	if len(number) < 8 || len(number) > 15 {
		return ""
	}
	return number
}

func whatsappKapsoNumberFromEnv() string {
	return sanitizeWhatsAppNumber(os.Getenv("WHATSAPP_KAPSO_NUMBER"))
}

func whatsappKapsoDefaultMessageFromEnv() string {
	return strings.TrimSpace(os.Getenv("WHATSAPP_KAPSO_DEFAULT_MESSAGE"))
}

func formatWhatsAppDisplayNumber(number string) string {
	if number == "" {
		return ""
	}
	return "+" + number
}

func whatsappKapsoChatURL(number, message string) string {
	number = sanitizeWhatsAppNumber(number)
	if number == "" {
		return ""
	}
	base := "https://wa.me/" + number
	if strings.TrimSpace(message) == "" {
		return base
	}
	query := url.Values{}
	query.Set("text", message)
	return base + "?" + query.Encode()
}

func normalizeWhatsAppLinkCode(raw string) string {
	compact := strings.ToUpper(strings.TrimSpace(raw))
	compact = strings.ReplaceAll(compact, "-", "")
	compact = strings.ReplaceAll(compact, " ", "")
	if len(compact) != whatsAppCodeLength {
		return ""
	}
	for _, r := range compact {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return compact
}

func formatWhatsAppLinkCode(compact string) string {
	if len(compact) != whatsAppCodeLength {
		return compact
	}
	return fmt.Sprintf("%s-%s", compact[:4], compact[4:])
}

func maskWhatsAppLinkCode(_ storage.WhatsAppLinkCode) string {
	return "****-****"
}

func hashWhatsAppLinkCode(compact string) string {
	sum := sha256.Sum256([]byte(compact))
	return hex.EncodeToString(sum[:])
}

func newWhatsAppLinkCode() (string, error) {
	var b strings.Builder
	for i := 0; i < whatsAppCodeLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(whatsAppCodeAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(whatsAppCodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func isUniqueViolationOnWhatsAppCodeHash(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return string(pqErr.Code) == "23505" && pqErr.Constraint == "whatsapp_link_codes_code_hash_key"
}

func buildWhatsAppLinkCommand(codeCompact string) string {
	return fmt.Sprintf("%s %s", whatsAppLinkCmdPrefix, formatWhatsAppLinkCode(codeCompact))
}

func (h *Handler) buildWhatsAppLinkStatusResponse(userID string, now time.Time) (whatsAppLinkStatusResponse, error) {
	premium, err := h.isUserPremium(userID)
	if err != nil {
		return whatsAppLinkStatusResponse{}, err
	}
	number := whatsappKapsoNumberFromEnv()
	response := whatsAppLinkStatusResponse{
		Provider:      "Kapso",
		Premium:       premium,
		Available:     premium && number != "",
		Linked:        false,
		Number:        number,
		NumberDisplay: formatWhatsAppDisplayNumber(number),
		ChatURL:       whatsappKapsoChatURL(number, whatsappKapsoDefaultMessageFromEnv()),
	}

	link, err := h.storage.GetWhatsAppUserLinkByUserID(userID)
	if err != nil && err != sql.ErrNoRows {
		return whatsAppLinkStatusResponse{}, err
	}
	if err == nil {
		response.Linked = true
		response.WhatsAppPhone = link.WhatsAppPhone
		response.LinkedAt = &link.CreatedAt
	}

	activeCode, err := h.storage.GetActiveWhatsAppLinkCode(userID, now)
	if err != nil && err != sql.ErrNoRows {
		return whatsAppLinkStatusResponse{}, err
	}
	if err == nil {
		response.ActiveCode = &whatsAppActiveCodeView{
			CodeMasked: maskWhatsAppLinkCode(activeCode),
			ExpiresAt:  activeCode.ExpiresAt,
		}
	}

	return response, nil
}

func (h *Handler) GetWhatsAppLinkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	response, err := h.buildWhatsAppLinkStatusResponse(userID, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo obtener el estado de WhatsApp"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) RefreshWhatsAppLinkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	response, err := h.buildWhatsAppLinkStatusResponse(userID, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo actualizar el estado de WhatsApp"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) CreateWhatsAppLinkCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	premium, err := h.isUserPremium(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo validar el plan actual"})
		return
	}
	if !premium {
		writeBotError(w, http.StatusForbidden, "Disponible solo para cuentas Premium", "premium_required")
		return
	}

	number := whatsappKapsoNumberFromEnv()
	if number == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "WHATSAPP_KAPSO_NUMBER no esta configurado"})
		return
	}

	now := time.Now().UTC()
	if err := h.storage.InvalidateActiveWhatsAppLinkCodes(userID, now); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo limpiar codigos previos"})
		return
	}

	var compactCode string
	var stored storage.WhatsAppLinkCode
	for attempts := 0; attempts < 5; attempts++ {
		compactCode, err = newWhatsAppLinkCode()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo generar codigo de vinculacion"})
			return
		}
		stored, err = h.storage.CreateWhatsAppLinkCode(
			userID,
			hashWhatsAppLinkCode(compactCode),
			now.Add(whatsAppLinkCodeTTL),
			now,
		)
		if err == nil {
			break
		}
		if !isUniqueViolationOnWhatsAppCodeHash(err) {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo guardar codigo de vinculacion"})
			return
		}
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo generar codigo unico"})
		return
	}

	chatURL := whatsappKapsoChatURL(number, whatsappKapsoDefaultMessageFromEnv())
	deepLinkURL := whatsappKapsoChatURL(number, buildWhatsAppLinkCommand(compactCode))
	writeJSON(w, http.StatusOK, whatsAppCreateCodeResponse{
		Code:          formatWhatsAppLinkCode(compactCode),
		ExpiresAt:     stored.ExpiresAt,
		ChatURL:       chatURL,
		DeepLinkURL:   deepLinkURL,
		NumberDisplay: formatWhatsAppDisplayNumber(number),
	})
}

func (h *Handler) GetWhatsAppKapsoContact(w http.ResponseWriter, r *http.Request) {
	// Backward compatible alias while clients migrate to /whatsapp/link-status.
	h.GetWhatsAppLinkStatus(w, r)
}
