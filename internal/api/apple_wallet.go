package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

const (
	walletSourceDefault            = "apple_wallet_shortcut"
	walletEventStatusReceived      = "received"
	walletEventStatusNeedsReview   = "needs_review"
	walletEventStatusDraftCreated  = "draft_transaction_created"
	walletEventStatusDuplicate     = "duplicate"
	walletEventStatusRejected      = "rejected"
	walletConfidenceLow            = "low"
	walletConfidenceMedium         = "medium"
	walletConfidenceHigh           = "high"
	walletDraftSystemOrigin        = "apple_wallet_shortcut_draft"
	walletDraftFallbackName        = "Apple Wallet purchase"
	walletDraftCategoryNeedsReview = "Por revisar"
	walletIngestTokenBytes         = 24
)

type appleWalletIngestRequest struct {
	Amount         float64         `json:"amount"`
	Merchant       string          `json:"merchant"`
	MerchantRaw    string          `json:"merchantRaw"`
	CardLabel      string          `json:"cardLabel"`
	WalletCategory string          `json:"walletCategory"`
	PaymentMethod  string          `json:"paymentMethod"`
	PaidAt         time.Time       `json:"paidAt"`
	Source         string          `json:"source"`
	RawPayload     json.RawMessage `json:"rawPayload"`
}

type appleWalletDebugResponse struct {
	Status  string `json:"status"`
	EventID string `json:"eventId"`
}

type appleWalletTokenStatusResponse struct {
	Premium   bool       `json:"premium"`
	HasToken  bool       `json:"has_token"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	LastUsed  *time.Time `json:"last_used_at,omitempty"`
}

type appleWalletCreateTokenResponse struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

type appleWalletIngestResponse struct {
	Status             string `json:"status"`
	EventID            string `json:"eventId"`
	TransactionID      string `json:"transactionId,omitempty"`
	DuplicateOfEventID string `json:"duplicateOfEventId,omitempty"`
	Result             string `json:"result,omitempty"`
	Message            string `json:"message,omitempty"`
	NextStep           string `json:"nextStep,omitempty"`
}

func normalizeMerchant(raw string) string {
	clean := storage.SanitizeString(raw)
	clean = strings.ToLower(strings.TrimSpace(clean))
	clean = strings.Join(strings.Fields(clean), " ")
	return clean
}

func normalizeWalletAmount(raw float64) float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 0
	}
	return math.Round(raw*100) / 100
}

func walletConfidenceForPayload(payload appleWalletIngestRequest, merchant string) string {
	if payload.Amount > 0 && merchant != "" && payload.PaidAt.IsZero() == false {
		return walletConfidenceHigh
	}
	if payload.Amount > 0 && (merchant != "" || (!payload.PaidAt.IsZero() && strings.TrimSpace(payload.CardLabel) != "")) {
		return walletConfidenceMedium
	}
	return walletConfidenceLow
}

func isWalletPayloadSufficient(payload appleWalletIngestRequest, merchant string) bool {
	if payload.Amount <= 0 {
		return false
	}
	if merchant != "" {
		return true
	}
	return strings.TrimSpace(payload.CardLabel) != "" && !payload.PaidAt.IsZero()
}

func normalizeWalletPaymentMethod(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CA":
		return "CA"
	case "TARJETA":
		return "TARJETA"
	case "EFECTIVO":
		return "EFECTIVO"
	default:
		return "CA"
	}
}

func hashWalletIngestToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newWalletIngestToken() (string, error) {
	buf := make([]byte, walletIngestTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func readShortcutBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func requestHeadersForDebug(r *http.Request) string {
	selected := map[string]string{
		"content_type":      strings.TrimSpace(r.Header.Get("Content-Type")),
		"user_agent":        strings.TrimSpace(r.Header.Get("User-Agent")),
		"x_forwarded_for":   strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
		"x_forwarded_proto": strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")),
	}
	encoded, _ := json.Marshal(selected)
	return string(encoded)
}

func decodeRequestBodyAsRawJSON(r *http.Request) (string, error) {
	defer r.Body.Close()
	var payload any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (h *Handler) AppleWalletDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	rawPayload, err := decodeRequestBodyAsRawJSON(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	event, err := h.storage.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:         userID,
		Source:         walletSourceDefault,
		RawPayload:     rawPayload,
		RequestHeaders: requestHeadersForDebug(r),
		Status:         walletEventStatusReceived,
		Confidence:     walletConfidenceLow,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to persist debug payload"})
		return
	}
	writeJSON(w, http.StatusCreated, appleWalletDebugResponse{Status: "ok", EventID: event.ID})
}

func (h *Handler) GetAppleWalletIngestTokenStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	premium, err := h.isUserPremium(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate plan"})
		return
	}
	if !premium {
		writeJSON(w, http.StatusOK, appleWalletTokenStatusResponse{Premium: false, HasToken: false})
		return
	}
	token, err := h.storage.GetActiveWalletIngestTokenByUserID(userID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, appleWalletTokenStatusResponse{Premium: true, HasToken: false})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load token status"})
		return
	}
	writeJSON(w, http.StatusOK, appleWalletTokenStatusResponse{
		Premium:   true,
		HasToken:  true,
		CreatedAt: &token.CreatedAt,
		LastUsed:  token.LastUsedAt,
	})
}

func (h *Handler) CreateAppleWalletIngestToken(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate plan"})
		return
	}
	if !premium {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Disponible solo para cuentas Premium",
			"code":  "premium_required",
		})
		return
	}
	token, err := newWalletIngestToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate token"})
		return
	}
	now := time.Now().UTC()
	stored, err := h.storage.UpsertWalletIngestToken(userID, hashWalletIngestToken(token), now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to persist token"})
		return
	}
	writeJSON(w, http.StatusOK, appleWalletCreateTokenResponse{
		Token:     token,
		CreatedAt: stored.CreatedAt,
	})
}

func (h *Handler) AppleWalletIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	rawToken := readShortcutBearerToken(r)
	if rawToken == "" {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}
	ingestToken, err := h.storage.GetWalletIngestTokenByHash(hashWalletIngestToken(rawToken))
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate token"})
		return
	}
	userID := ingestToken.UserID
	premium, err := h.isUserPremium(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate plan"})
		return
	}
	if !premium {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Premium requerido",
			"code":  "premium_required",
		})
		return
	}
	_ = h.storage.TouchWalletIngestTokenLastUsed(ingestToken.ID, time.Now().UTC())
	rawPayload, err := decodeRequestBodyAsRawJSON(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	var payload appleWalletIngestRequest
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	payload.Amount = normalizeWalletAmount(payload.Amount)
	merchantNormalized := normalizeMerchant(payload.Merchant)
	if merchantNormalized == "" {
		merchantNormalized = normalizeMerchant(payload.MerchantRaw)
	}
	status := walletEventStatusReceived
	confidence := walletConfidenceForPayload(payload, merchantNormalized)
	if !isWalletPayloadSufficient(payload, merchantNormalized) {
		status = walletEventStatusNeedsReview
	}
	if status != walletEventStatusNeedsReview && payload.PaidAt.IsZero() {
		payload.PaidAt = time.Now().UTC()
	}
	if strings.TrimSpace(payload.Source) == "" {
		payload.Source = walletSourceDefault
	}
	paymentMethod := normalizeWalletPaymentMethod(payload.PaymentMethod)
	event, err := h.storage.CreateWalletIngestEvent(storage.WalletIngestEvent{
		UserID:         userID,
		Source:         payload.Source,
		Amount:         payload.Amount,
		Merchant:       merchantNormalized,
		MerchantRaw:    strings.TrimSpace(payload.MerchantRaw),
		CardLabel:      strings.TrimSpace(payload.CardLabel),
		WalletCategory: strings.TrimSpace(payload.WalletCategory),
		PaidAt:         payload.PaidAt,
		RawPayload:     rawPayload,
		RequestHeaders: requestHeadersForDebug(r),
		Status:         status,
		Confidence:     confidence,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to persist ingest event"})
		return
	}

	if status == walletEventStatusNeedsReview {
		writeJSON(w, http.StatusAccepted, appleWalletIngestResponse{
			Status:   status,
			EventID:  event.ID,
			Result:   "queued_for_review",
			Message:  "Evento recibido, pero faltan datos para crear la transaccion automaticamente.",
			NextStep: "Revisa y completa la transaccion en ExpenseLog.",
		})
		return
	}

	dup, err := h.storage.FindPotentialDuplicateWalletIngestEvent(userID, payload.Amount, merchantNormalized, payload.PaidAt, 10*time.Minute)
	if err == nil && dup.ID != "" && dup.ID != event.ID {
		if updateErr := h.storage.UpdateWalletIngestEventResult(event.ID, walletEventStatusDuplicate, confidence, "", dup.ID); updateErr != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update duplicate event"})
			return
		}
		writeJSON(w, http.StatusOK, appleWalletIngestResponse{
			Status:             walletEventStatusDuplicate,
			EventID:            event.ID,
			DuplicateOfEventID: dup.ID,
			Result:             "duplicate_ignored",
			Message:            "Evento duplicado detectado. No se creo una nueva transaccion.",
			NextStep:           "No requiere accion.",
		})
		return
	}
	if err != nil && err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to inspect duplicates"})
		return
	}

	currency, err := h.storage.GetCurrency(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to resolve currency"})
		return
	}
	draftName := strings.TrimSpace(payload.Merchant)
	if draftName == "" {
		draftName = strings.TrimSpace(payload.MerchantRaw)
	}
	if draftName == "" {
		draftName = walletDraftFallbackName
	}
	flow, amount, err := normalizeFlow("expense", payload.Amount)
	if err != nil {
		_ = h.storage.UpdateWalletIngestEventResult(event.ID, walletEventStatusRejected, confidence, "", "")
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid amount"})
		return
	}
	expense := storage.Expense{
		ID:           uuid.New().String(),
		Name:         draftName,
		Category:     walletDraftCategoryNeedsReview,
		Amount:       amount,
		Currency:     currency,
		Flow:         flow,
		Source:       paymentMethod,
		Card:         strings.TrimSpace(payload.CardLabel),
		Date:         payload.PaidAt,
		SystemOrigin: walletDraftSystemOrigin,
	}
	if err := expense.Validate(); err != nil {
		_ = h.storage.UpdateWalletIngestEventResult(event.ID, walletEventStatusRejected, confidence, "", "")
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.storage.AddExpense(userID, expense); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create draft transaction"})
		return
	}
	if err := h.storage.UpdateWalletIngestEventResult(event.ID, walletEventStatusDraftCreated, confidence, expense.ID, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update ingest event"})
		return
	}
	writeJSON(w, http.StatusCreated, appleWalletIngestResponse{
		Status:        walletEventStatusDraftCreated,
		EventID:       event.ID,
		TransactionID: expense.ID,
		Result:        "transaction_created",
		Message:       "Transaccion creada automaticamente como borrador para revision.",
		NextStep:      "Abre ExpenseLog para revisar categoria, metodo y monto.",
	})
}
