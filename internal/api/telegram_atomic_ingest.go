package api

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const telegramAtomicIngestTTL = 3 * time.Minute

type telegramAtomicIngestState struct {
	MergedPayload botExpensePayload
	SeenCaption   bool
	SeenMedia     bool
	DoneUntil     time.Time
	UpdatedAt     time.Time
}

var telegramAtomicIngestBuffer = struct {
	mu    sync.Mutex
	items map[string]telegramAtomicIngestState
}{
	items: make(map[string]telegramAtomicIngestState),
}

func isTelegramAttachmentAtomicIngestEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_ATTACHMENT_ATOMIC_INTAKE_ENABLED"))
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

func (h *Handler) resolveTelegramAtomicIngestPayload(payload botExpensePayload, now time.Time) (botExpensePayload, bool, bool) {
	if !isTelegramAttachmentAtomicIngestEnabled() {
		return payload, true, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	key, part := extractTelegramAtomicIngestKeyAndPart(payload)
	if key == "" {
		return payload, true, false
	}
	bufferKey := strings.TrimSpace(strings.ToLower(key)) + "|" + stringifyInt64(payload.TelegramUserID)

	telegramAtomicIngestBuffer.mu.Lock()
	defer telegramAtomicIngestBuffer.mu.Unlock()

	cleanupTelegramAtomicIngestLocked(now)

	state, exists := telegramAtomicIngestBuffer.items[bufferKey]
	if exists && !state.DoneUntil.IsZero() && state.DoneUntil.After(now) {
		return botExpensePayload{}, false, true
	}

	if !exists || (!state.DoneUntil.IsZero() && state.DoneUntil.Before(now)) {
		state = telegramAtomicIngestState{}
	}
	state.MergedPayload = mergeBotExpensePayload(state.MergedPayload, payload)
	state.UpdatedAt = now

	switch part {
	case "caption":
		state.SeenCaption = true
	case "media":
		state.SeenMedia = true
	case "complete":
		state.SeenCaption = true
		state.SeenMedia = true
	default:
		state.SeenMedia = true
	}

	ready := state.SeenCaption && state.SeenMedia
	if !ready {
		telegramAtomicIngestBuffer.items[bufferKey] = state
		return botExpensePayload{}, false, false
	}

	resolved := state.MergedPayload
	state.DoneUntil = now.Add(telegramAtomicIngestTTL)
	state.MergedPayload = botExpensePayload{}
	state.SeenCaption = false
	state.SeenMedia = false
	telegramAtomicIngestBuffer.items[bufferKey] = state
	return resolved, true, false
}

func cleanupTelegramAtomicIngestLocked(now time.Time) {
	for key, state := range telegramAtomicIngestBuffer.items {
		expiredPending := !state.DoneUntil.IsZero() && now.After(state.DoneUntil)
		expiredOpen := state.DoneUntil.IsZero() && now.Sub(state.UpdatedAt) > telegramAtomicIngestTTL
		if expiredPending || expiredOpen {
			delete(telegramAtomicIngestBuffer.items, key)
		}
	}
}

func extractTelegramAtomicIngestKeyAndPart(payload botExpensePayload) (string, string) {
	flat := flattenTelegramSourceMeta(payload.SourceMeta)
	keyCandidates := []string{
		flat["ingest_key"],
		flat["message_id"],
		flat["telegram_message_id"],
		flat["media_group_id"],
		flat["update_id"],
		flat["event_id"],
	}
	for _, candidate := range keyCandidates {
		clean := strings.TrimSpace(candidate)
		if clean != "" {
			part := strings.ToLower(strings.TrimSpace(flat["ingest_part"]))
			if part == "" {
				part = inferTelegramAtomicPart(payload)
			}
			return clean, part
		}
	}
	return "", "complete"
}

func inferTelegramAtomicPart(payload botExpensePayload) string {
	if payload.Amount == 0 && strings.TrimSpace(payload.Type) == "" {
		hasText := strings.TrimSpace(payload.Counterparty) != "" ||
			strings.TrimSpace(payload.Reference) != "" ||
			strings.TrimSpace(payload.Motive) != ""
		if hasText {
			return "caption"
		}
	}
	if payload.Amount != 0 || strings.TrimSpace(payload.Type) != "" {
		return "media"
	}
	return "complete"
}

func flattenTelegramSourceMeta(raw json.RawMessage) map[string]string {
	out := make(map[string]string, 16)
	if len(raw) == 0 {
		return out
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return out
	}
	collectSourceMetaLeaves("", decoded, out)
	normalized := make(map[string]string, len(out))
	for key, value := range out {
		normalized[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return normalized
}

func mergeBotExpensePayload(base, incoming botExpensePayload) botExpensePayload {
	out := base
	if incoming.TelegramUserID > 0 {
		out.TelegramUserID = incoming.TelegramUserID
	}
	if strings.TrimSpace(incoming.Type) != "" {
		out.Type = incoming.Type
	}
	if incoming.Amount != 0 {
		out.Amount = incoming.Amount
	}
	if strings.TrimSpace(incoming.Currency) != "" {
		out.Currency = incoming.Currency
	}
	if strings.TrimSpace(incoming.DateTimeISO) != "" {
		out.DateTimeISO = incoming.DateTimeISO
	}
	if strings.TrimSpace(incoming.Counterparty) != "" {
		out.Counterparty = incoming.Counterparty
	}
	if strings.TrimSpace(incoming.Reference) != "" {
		out.Reference = incoming.Reference
	}
	if strings.TrimSpace(incoming.Motive) != "" {
		out.Motive = incoming.Motive
	}
	if strings.TrimSpace(incoming.Category) != "" {
		out.Category = incoming.Category
	}
	if strings.TrimSpace(incoming.Provider) != "" {
		out.Provider = incoming.Provider
	}
	if len(incoming.Tags) > 0 {
		out.Tags = append([]string(nil), incoming.Tags...)
	}
	if len(incoming.SourceMeta) > 0 {
		out.SourceMeta = incoming.SourceMeta
	}
	return out
}

func stringifyInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
