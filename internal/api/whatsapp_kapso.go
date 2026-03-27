package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

type whatsappKapsoContactResponse struct {
	Provider      string `json:"provider"`
	Premium       bool   `json:"premium"`
	Available     bool   `json:"available"`
	Number        string `json:"number,omitempty"`
	NumberDisplay string `json:"number_display,omitempty"`
	ChatURL       string `json:"chat_url,omitempty"`
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

func (h *Handler) GetWhatsAppKapsoContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	planTier, err := h.storage.GetUserPlanTier(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user plan"})
		return
	}
	premium := storage.NormalizePlanTier(planTier) == storage.PlanTierPremium

	number := whatsappKapsoNumberFromEnv()
	response := whatsappKapsoContactResponse{
		Provider:  "Kapso",
		Premium:   premium,
		Available: premium && number != "",
	}

	if premium && number != "" {
		response.Number = number
		response.NumberDisplay = formatWhatsAppDisplayNumber(number)
		response.ChatURL = whatsappKapsoChatURL(number, whatsappKapsoDefaultMessageFromEnv())
	}

	writeJSON(w, http.StatusOK, response)
}
