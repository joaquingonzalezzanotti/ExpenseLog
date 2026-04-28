package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/botcore"
	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

func isUnifiedDecisionEngineEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("UNIFIED_DECISION_ENGINE_ENABLED"))
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

func (h *Handler) buildIdentityProfileForUser(userID string) botcore.IdentityProfile {
	aliases, err := h.storage.GetTelegramIdentityAliases(userID)
	if err != nil {
		aliases = nil
	}
	fingerprints, err := h.storage.GetTelegramOwnedAccountFingerprints(userID)
	if err != nil {
		fingerprints = nil
	}

	profile := botcore.IdentityProfile{
		Aliases:       make([]botcore.IdentityAlias, 0, len(aliases)),
		OwnedAccounts: make([]botcore.OwnedAccountFingerprint, 0, len(fingerprints)),
	}
	for _, alias := range aliases {
		profile.Aliases = append(profile.Aliases, botcore.IdentityAlias{
			AliasNorm:  alias.AliasNorm,
			Confidence: alias.Confidence,
		})
	}
	for _, fp := range fingerprints {
		profile.OwnedAccounts = append(profile.OwnedAccounts, botcore.OwnedAccountFingerprint{
			AccountLast4: fp.AccountLast4,
			CBUCVULast4:  fp.CBUCVULast4,
			Confidence:   fp.Confidence,
		})
	}
	return profile
}

func buildTelegramParseCandidate(payload botExpensePayload, now time.Time) botcore.ParseCandidate {
	sourceLast4, destinationLast4 := extractTelegramAccountHints(payload.SourceMeta)
	flatMeta := flattenTelegramSourceMeta(payload.SourceMeta)
	rawText := firstNonEmpty(
		flatMeta["raw_text"],
		flatMeta["text"],
		flatMeta["message_text"],
		flatMeta["ocr_text"],
	)
	mediaCaption := firstNonEmpty(
		flatMeta["media_caption"],
		flatMeta["caption"],
	)
	return botcore.ParseCandidate{
		Channel:      botcore.ChannelTelegram,
		FlowHint:     payload.Type,
		Amount:       payload.Amount,
		Currency:     payload.Currency,
		DateTime:     parseTelegramDateTime(payload.DateTimeISO, now),
		Counterparty: payload.Counterparty,
		Reference:    payload.Reference,
		Motive:       payload.Motive,
		CategoryHint: payload.Category,
		SourceHint:   payload.Provider,
		Tags:         payload.Tags,
		Evidence: botcore.ParseEvidence{
			SourceAccountLast4:      sourceLast4,
			DestinationAccountLast4: destinationLast4,
			RawText:                 strings.TrimSpace(rawText),
			MediaCaption:            strings.TrimSpace(mediaCaption),
		},
	}
}

func buildWhatsAppParseCandidate(parsed whatsAppParsedExpense) botcore.ParseCandidate {
	return botcore.ParseCandidate{
		Channel:      botcore.ChannelWhatsApp,
		FlowHint:     parsed.Flow,
		Amount:       parsed.Amount,
		Currency:     parsed.Currency,
		DateTime:     parsed.Date,
		Counterparty: parsed.Name,
		CategoryHint: parsed.Category,
		SourceHint:   parsed.Source,
	}
}

func (h *Handler) decideFromCandidate(userID string, candidate botcore.ParseCandidate, defaultCurrency string, now time.Time) (botcore.Decision, error) {
	return botcore.Decide(botcore.DecisionRequest{
		Candidate:       candidate,
		Identity:        h.buildIdentityProfileForUser(userID),
		DefaultCurrency: defaultCurrency,
		Now:             now,
	})
}

func extractTelegramAccountHints(raw json.RawMessage) (sourceLast4 string, destinationLast4 string) {
	if len(raw) == 0 {
		return "", ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", ""
	}
	flattened := make(map[string]string, 16)
	collectSourceMetaLeaves("", decoded, flattened)
	for key, value := range flattened {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		last4 := extractLast4Digits(value)
		if last4 == "" {
			continue
		}
		if sourceLast4 == "" && isSourceAccountHintKey(lowerKey) {
			sourceLast4 = last4
		}
		if destinationLast4 == "" && isDestinationAccountHintKey(lowerKey) {
			destinationLast4 = last4
		}
	}
	return sourceLast4, destinationLast4
}

func collectSourceMetaLeaves(prefix string, node any, out map[string]string) {
	switch item := node.(type) {
	case map[string]any:
		for key, value := range item {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			collectSourceMetaLeaves(next, value, out)
		}
	case []any:
		for idx, value := range item {
			next := prefix
			if next == "" {
				next = "item"
			}
			collectSourceMetaLeaves(next+"["+strconv.Itoa(idx)+"]", value, out)
		}
	case string:
		out[prefix] = item
	}
}

func isSourceAccountHintKey(key string) bool {
	return strings.Contains(key, "from") ||
		strings.Contains(key, "source") ||
		strings.Contains(key, "origen") ||
		strings.Contains(key, "desde") ||
		strings.Contains(key, "emisor")
}

func isDestinationAccountHintKey(key string) bool {
	return strings.Contains(key, "to") ||
		strings.Contains(key, "dest") ||
		strings.Contains(key, "para") ||
		strings.Contains(key, "receptor") ||
		strings.Contains(key, "beneficiario")
}

func extractLast4Digits(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return ""
	}
	if len(digits) <= 4 {
		return digits
	}
	return digits[len(digits)-4:]
}

func decisionToExpense(decision botcore.Decision, tags []string, systemOrigin string) storage.Expense {
	return storage.Expense{
		ID:           uuid.New().String(),
		Flow:         decision.Flow,
		Name:         decision.Name,
		Category:     decision.Category,
		Amount:       decision.Amount,
		Currency:     decision.Currency,
		Source:       decision.Source,
		Tags:         uniqueTags(tags),
		SystemOrigin: systemOrigin,
		Date:         decision.DateTime,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func persistExpensesWithRollback(s storage.Storage, userID string, expenses []storage.Expense) error {
	if len(expenses) == 0 {
		return nil
	}
	insertedIDs := make([]string, 0, len(expenses))
	for _, expense := range expenses {
		if err := s.AddExpense(userID, expense); err != nil {
			if len(insertedIDs) > 0 {
				_ = s.RemoveMultipleExpenses(userID, insertedIDs)
			}
			return err
		}
		insertedIDs = append(insertedIDs, expense.ID)
	}
	return nil
}

func extractTelegramBatchPayloads(payload botExpensePayload) []botExpensePayload {
	if len(payload.SourceMeta) == 0 {
		return nil
	}
	var envelope struct {
		BatchItems []json.RawMessage `json:"batch_items"`
	}
	if err := json.Unmarshal(payload.SourceMeta, &envelope); err != nil {
		return nil
	}
	if len(envelope.BatchItems) == 0 {
		return nil
	}

	items := make([]botExpensePayload, 0, len(envelope.BatchItems))
	for _, raw := range envelope.BatchItems {
		var item botExpensePayload
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		item.TelegramUserID = payload.TelegramUserID
		if strings.TrimSpace(item.Currency) == "" {
			item.Currency = payload.Currency
		}
		if strings.TrimSpace(item.Provider) == "" {
			item.Provider = payload.Provider
		}
		if strings.TrimSpace(item.DateTimeISO) == "" {
			item.DateTimeISO = payload.DateTimeISO
		}
		if len(item.Tags) == 0 && len(payload.Tags) > 0 {
			item.Tags = append([]string(nil), payload.Tags...)
		}
		if len(item.SourceMeta) == 0 {
			item.SourceMeta = payload.SourceMeta
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func summarizeTelegramBatchResult(expenses []storage.Expense) string {
	if len(expenses) == 0 {
		return "No se registraron movimientos."
	}
	var income float64
	var expense float64
	for _, item := range expenses {
		if item.Amount > 0 {
			income += item.Amount
		} else {
			expense += -item.Amount
		}
	}
	currency := strings.ToUpper(strings.TrimSpace(expenses[0].Currency))
	if currency == "" {
		currency = "ARS"
	}
	net := income - expense
	return fmt.Sprintf("Batch registrado: %d movimientos. Ingresos %.2f %s | Gastos %.2f %s | Neto %.2f %s", len(expenses), income, currency, expense, currency, net, currency)
}

func (h *Handler) createBotPendingDecision(userID, channel, subjectKey string, candidate botcore.ParseCandidate, defaultCurrency string, now time.Time) (storage.BotPendingDecision, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return storage.BotPendingDecision{}, err
	}
	return h.storage.CreateBotPendingDecision(storage.BotPendingDecision{
		UserID:          userID,
		Channel:         strings.TrimSpace(strings.ToLower(channel)),
		SubjectKey:      strings.TrimSpace(strings.ToLower(subjectKey)),
		CandidateJSON:   string(candidateJSON),
		DefaultCurrency: strings.ToLower(strings.TrimSpace(defaultCurrency)),
		Status:          "pending",
		CreatedAt:       now,
		ExpiresAt:       now.Add(15 * time.Minute),
	})
}

func buildTelegramDecisionSubjectKey(payload botExpensePayload) string {
	flat := flattenTelegramSourceMeta(payload.SourceMeta)
	key := firstNonEmpty(
		flat["ingest_key"],
		flat["message_id"],
		flat["telegram_message_id"],
		flat["media_group_id"],
		flat["update_id"],
		flat["event_id"],
	)
	if key != "" {
		return "tg:" + strings.ToLower(strings.TrimSpace(key))
	}
	raw := strings.ToLower(strings.TrimSpace(payload.Type)) + "|" +
		strings.TrimSpace(payload.DateTimeISO) + "|" +
		strings.TrimSpace(payload.Counterparty) + "|" +
		strconv.FormatFloat(payload.Amount, 'f', 2, 64)
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		raw = "tg:fallback"
	}
	return raw
}

func (h *Handler) recordBotDecisionEvent(userID string, candidate botcore.ParseCandidate, decision botcore.Decision, now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	channel := strings.TrimSpace(string(candidate.Channel))
	_ = h.storage.AddBotDecisionEvent(userID, storage.BotDecisionEvent{
		Channel:      channel,
		Decision:     decision.Flow,
		Amount:       decision.Amount,
		Currency:     decision.Currency,
		Confidence:   decision.Confidence,
		Ambiguous:    decision.Ambiguous,
		Reasons:      append([]string(nil), decision.Reasons...),
		RawText:      strings.TrimSpace(candidate.Evidence.RawText),
		MediaCaption: strings.TrimSpace(candidate.Evidence.MediaCaption),
		CreatedAt:    now,
	})
}
