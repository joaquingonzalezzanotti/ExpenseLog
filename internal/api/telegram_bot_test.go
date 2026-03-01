package api

import (
	"testing"
	"time"
)

func TestNormalizeTelegramLinkCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "abcd-1234", want: "ABCD1234"},
		{input: "ab cd 12 34", want: "ABCD1234"},
		{input: "ABCD1234", want: "ABCD1234"},
		{input: "AB12", want: ""},
		{input: "ABC#1234", want: ""},
	}
	for _, tt := range tests {
		got := normalizeTelegramLinkCode(tt.input)
		if got != tt.want {
			t.Fatalf("normalizeTelegramLinkCode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseTelegramDateTime(t *testing.T) {
	fallback := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	gotRFC := parseTelegramDateTime("2026-03-01T11:15:00-03:00", fallback)
	if gotRFC.Year() != 2026 || gotRFC.Month() != time.March || gotRFC.Day() != 1 {
		t.Fatalf("expected RFC3339 parse to keep date, got %v", gotRFC)
	}

	gotAR := parseTelegramDateTime("01/03/2026 19:45", fallback)
	if gotAR.Year() != 2026 || gotAR.Month() != time.March || gotAR.Day() != 1 || gotAR.Hour() != 19 {
		t.Fatalf("expected AR parse to work, got %v", gotAR)
	}

	gotFallback := parseTelegramDateTime("invalid", fallback)
	if !gotFallback.Equal(fallback) {
		t.Fatalf("expected fallback on invalid datetime, got %v", gotFallback)
	}
}

func TestFormatTelegramLinkCode(t *testing.T) {
	got := formatTelegramLinkCode("ABCDEFG1")
	if got != "ABCD-EFG1" {
		t.Fatalf("formatTelegramLinkCode returned %q", got)
	}
}

func TestSanitizeTelegramBotUsername(t *testing.T) {
	if got := sanitizeTelegramBotUsername("@ExpenseLogBot"); got != "ExpenseLogBot" {
		t.Fatalf("sanitizeTelegramBotUsername returned %q", got)
	}
	if got := sanitizeTelegramBotUsername("ab"); got != "" {
		t.Fatalf("expected invalid short username to be rejected, got %q", got)
	}
}
