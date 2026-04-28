package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResolveTelegramAtomicIngestPayloadDisabled(t *testing.T) {
	t.Setenv("TELEGRAM_ATTACHMENT_ATOMIC_INTAKE_ENABLED", "false")
	h := &Handler{}
	payload := botExpensePayload{
		TelegramUserID: 10,
		Type:           "expense",
		Amount:         1000,
	}
	resolved, ready, duplicate := h.resolveTelegramAtomicIngestPayload(payload, time.Now().UTC())
	if !ready || duplicate {
		t.Fatalf("expected ready=true duplicate=false, got ready=%v duplicate=%v", ready, duplicate)
	}
	if resolved.Amount != 1000 {
		t.Fatalf("unexpected resolved payload amount: %.2f", resolved.Amount)
	}
}

func TestResolveTelegramAtomicIngestPayloadMergesCaptionAndMedia(t *testing.T) {
	t.Setenv("TELEGRAM_ATTACHMENT_ATOMIC_INTAKE_ENABLED", "true")
	telegramAtomicIngestBuffer.mu.Lock()
	telegramAtomicIngestBuffer.items = make(map[string]telegramAtomicIngestState)
	telegramAtomicIngestBuffer.mu.Unlock()

	h := &Handler{}
	now := time.Now().UTC()
	captionMeta, _ := json.Marshal(map[string]any{
		"ingest_key":  "msg-1",
		"ingest_part": "caption",
	})
	captionPayload := botExpensePayload{
		TelegramUserID: 10,
		Motive:         "VAR",
		SourceMeta:     captionMeta,
	}
	_, readyCaption, duplicateCaption := h.resolveTelegramAtomicIngestPayload(captionPayload, now)
	if readyCaption || duplicateCaption {
		t.Fatalf("expected caption part to be pending, got ready=%v duplicate=%v", readyCaption, duplicateCaption)
	}

	mediaMeta, _ := json.Marshal(map[string]any{
		"ingest_key":  "msg-1",
		"ingest_part": "media",
	})
	mediaPayload := botExpensePayload{
		TelegramUserID: 10,
		Type:           "expense",
		Amount:         30000,
		Currency:       "ars",
		SourceMeta:     mediaMeta,
	}
	resolved, readyMedia, duplicateMedia := h.resolveTelegramAtomicIngestPayload(mediaPayload, now.Add(time.Second))
	if !readyMedia || duplicateMedia {
		t.Fatalf("expected ready=true duplicate=false for merged payload, got ready=%v duplicate=%v", readyMedia, duplicateMedia)
	}
	if resolved.Amount != 30000 || resolved.Type != "expense" {
		t.Fatalf("unexpected resolved core fields: %+v", resolved)
	}
	if resolved.Motive != "VAR" {
		t.Fatalf("expected motive to be merged from caption part, got %q", resolved.Motive)
	}
}
