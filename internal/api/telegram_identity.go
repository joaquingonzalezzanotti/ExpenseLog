package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

type telegramIdentityAliasPayload struct {
	Alias      string  `json:"alias"`
	Confidence float64 `json:"confidence"`
}

type telegramOwnedAccountFingerprintPayload struct {
	BankNorm     string  `json:"bank_norm"`
	AccountLast4 string  `json:"account_last4"`
	CBUCVULast4  string  `json:"cbu_cvu_last4"`
	HolderNorm   string  `json:"holder_norm"`
	Confidence   float64 `json:"confidence"`
}

type telegramIdentityPayload struct {
	Aliases      []telegramIdentityAliasPayload           `json:"aliases"`
	Fingerprints []telegramOwnedAccountFingerprintPayload `json:"fingerprints"`
}

type telegramIdentityResponse struct {
	Enabled      bool                                     `json:"enabled"`
	Premium      bool                                     `json:"premium"`
	Linked       bool                                     `json:"linked"`
	Aliases      []telegramIdentityAliasPayload           `json:"aliases"`
	Fingerprints []telegramOwnedAccountFingerprintPayload `json:"fingerprints"`
}

type botTelegramIdentityRequest struct {
	TelegramUserID int64 `json:"telegram_user_id"`
}

func isTelegramIdentityV2Enabled() bool {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_IDENTITY_V2_ENABLED"))
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func mapAliasModelToPayload(aliases []storage.TelegramIdentityAlias) []telegramIdentityAliasPayload {
	out := make([]telegramIdentityAliasPayload, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, telegramIdentityAliasPayload{
			Alias:      alias.AliasRaw,
			Confidence: alias.Confidence,
		})
	}
	return out
}

func mapFingerprintModelToPayload(items []storage.TelegramOwnedAccountFingerprint) []telegramOwnedAccountFingerprintPayload {
	out := make([]telegramOwnedAccountFingerprintPayload, 0, len(items))
	for _, fp := range items {
		out = append(out, telegramOwnedAccountFingerprintPayload{
			BankNorm:     fp.BankNorm,
			AccountLast4: fp.AccountLast4,
			CBUCVULast4:  fp.CBUCVULast4,
			HolderNorm:   fp.HolderNorm,
			Confidence:   fp.Confidence,
		})
	}
	return out
}

func (h *Handler) getTelegramIdentityStateForUser(userID string) (telegramIdentityResponse, error) {
	status, err := h.buildTelegramLinkStatusResponse(userID, time.Now().UTC())
	if err != nil {
		return telegramIdentityResponse{}, err
	}
	response := telegramIdentityResponse{
		Enabled: isTelegramIdentityV2Enabled(),
		Premium: status.Premium,
		Linked:  status.Linked,
	}
	if !status.Premium || !status.Linked {
		return response, nil
	}

	aliases, err := h.storage.GetTelegramIdentityAliases(userID)
	if err != nil {
		return telegramIdentityResponse{}, err
	}
	fingerprints, err := h.storage.GetTelegramOwnedAccountFingerprints(userID)
	if err != nil {
		return telegramIdentityResponse{}, err
	}
	response.Aliases = mapAliasModelToPayload(aliases)
	response.Fingerprints = mapFingerprintModelToPayload(fingerprints)
	return response, nil
}

func (h *Handler) TelegramIdentity(w http.ResponseWriter, r *http.Request) {
	if !isTelegramIdentityV2Enabled() {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Feature not enabled"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		response, err := h.getTelegramIdentityStateForUser(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo obtener identidad de Telegram"})
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPut:
		state, err := h.getTelegramIdentityStateForUser(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo validar estado de Telegram"})
			return
		}
		if !state.Premium {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "Disponible solo para cuentas Premium"})
			return
		}
		if !state.Linked {
			writeJSON(w, http.StatusPreconditionFailed, ErrorResponse{Error: "Primero vincula Telegram para configurar identidad"})
			return
		}

		var payload telegramIdentityPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
			return
		}
		now := time.Now().UTC()
		aliases := make([]storage.TelegramIdentityAlias, 0, len(payload.Aliases))
		for _, input := range payload.Aliases {
			aliases = append(aliases, storage.TelegramIdentityAlias{
				AliasRaw:   input.Alias,
				Confidence: input.Confidence,
				Source:     "user_settings",
			})
		}
		fingerprints := make([]storage.TelegramOwnedAccountFingerprint, 0, len(payload.Fingerprints))
		for _, input := range payload.Fingerprints {
			fingerprints = append(fingerprints, storage.TelegramOwnedAccountFingerprint{
				BankNorm:     input.BankNorm,
				AccountLast4: input.AccountLast4,
				CBUCVULast4:  input.CBUCVULast4,
				HolderNorm:   input.HolderNorm,
				Confidence:   input.Confidence,
				Source:       "user_settings",
			})
		}
		if err := h.storage.ReplaceTelegramIdentityAliases(userID, aliases, now); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo actualizar aliases"})
			return
		}
		if err := h.storage.ReplaceTelegramOwnedAccountFingerprints(userID, fingerprints, now); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo actualizar cuentas propias"})
			return
		}
		response, err := h.getTelegramIdentityStateForUser(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "No se pudo obtener identidad actualizada"})
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
	}
}

func (h *Handler) GetBotTelegramIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeBotError(w, http.StatusMethodNotAllowed, "Method not allowed", "method_not_allowed")
		return
	}
	if !isTelegramIdentityV2Enabled() {
		writeBotError(w, http.StatusNotFound, "Feature not enabled", "feature_disabled")
		return
	}
	var payload botTelegramIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeBotError(w, http.StatusBadRequest, "Invalid request body", "invalid_payload")
		return
	}
	if payload.TelegramUserID <= 0 {
		writeBotError(w, http.StatusBadRequest, "telegram_user_id is required", "invalid_payload")
		return
	}

	link, err := h.storage.GetTelegramUserLinkByTelegramUserID(payload.TelegramUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, telegramIdentityResponse{
				Enabled: isTelegramIdentityV2Enabled(),
				Premium: false,
				Linked:  false,
			})
			return
		}
		writeBotError(w, http.StatusInternalServerError, "No se pudo resolver vinculacion de Telegram", "internal_error")
		return
	}
	state, err := h.getTelegramIdentityStateForUser(link.UserID)
	if err != nil {
		writeBotError(w, http.StatusInternalServerError, "No se pudo obtener identidad de Telegram", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, state)
}
