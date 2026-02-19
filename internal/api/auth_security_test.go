package api

import (
	"net/http"
	"testing"
)

func TestReadClientIPIgnoresForwardedHeadersWhenUntrusted(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	req, err := http.NewRequest(http.MethodGet, "http://example.test/app", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.RemoteAddr = "203.0.113.9:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 10.0.0.1")

	got := readClientIP(req)
	if got != "203.0.113.9" {
		t.Fatalf("expected remote host IP, got %q", got)
	}
}

func TestReadClientIPUsesForwardedHeadersWhenTrusted(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "true")
	req, err := http.NewRequest(http.MethodGet, "http://example.test/app", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.RemoteAddr = "203.0.113.9:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 10.0.0.1")

	got := readClientIP(req)
	if got != "198.51.100.5" {
		t.Fatalf("expected forwarded IP, got %q", got)
	}
}

func TestExternalBaseURLUsesConfiguredAppBaseURL(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://www.expenselog.com.ar/")
	req, err := http.NewRequest(http.MethodGet, "https://ignored.test/app", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "ignored.test"

	base, baseErr := externalBaseURL(req)
	if baseErr != nil {
		t.Fatalf("unexpected error: %v", baseErr)
	}
	if base != "https://www.expenselog.com.ar" {
		t.Fatalf("unexpected base url: %q", base)
	}
}

func TestExternalBaseURLRequiresConfigForPublicHost(t *testing.T) {
	t.Setenv("APP_BASE_URL", "")
	req, err := http.NewRequest(http.MethodGet, "https://expenselog-dev.up.railway.app/app", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "expenselog-dev.up.railway.app"

	_, baseErr := externalBaseURL(req)
	if baseErr == nil {
		t.Fatalf("expected error when APP_BASE_URL is missing on public host")
	}
}

func TestExternalBaseURLAllowsLocalFallback(t *testing.T) {
	t.Setenv("APP_BASE_URL", "")
	req, err := http.NewRequest(http.MethodGet, "http://localhost:8080/app", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "localhost:8080"

	base, baseErr := externalBaseURL(req)
	if baseErr != nil {
		t.Fatalf("unexpected error: %v", baseErr)
	}
	if base != "http://localhost:8080" {
		t.Fatalf("unexpected local fallback base url: %q", base)
	}
}
