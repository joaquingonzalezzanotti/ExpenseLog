package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeStaticFile_OnboardingAssets(t *testing.T) {
	h := &Handler{}
	tests := []struct {
		name         string
		path         string
		wantContains string
	}{
		{
			name:         "onboarding css",
			path:         "/app/onboarding.css",
			wantContains: "text/css",
		},
		{
			name:         "onboarding js",
			path:         "/app/onboarding_ui.js",
			wantContains: "application/javascript",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()

			h.ServeStaticFile(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
			}
			contentType := rec.Header().Get("Content-Type")
			if !strings.Contains(contentType, tc.wantContains) {
				t.Fatalf("unexpected content-type: got %q want contain %q", contentType, tc.wantContains)
			}
		})
	}
}

func TestServeStaticFile_NotFound(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/app/does-not-exist.css", nil)
	rec := httptest.NewRecorder()

	h.ServeStaticFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServeStaticFile_RobotsIncludesDynamicSitemapURL(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://expense.example")
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()

	h.ServeStaticFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Disallow: /api/") {
		t.Fatalf("expected robots to disallow /api/, got %q", body)
	}
	if !strings.Contains(body, "Sitemap: https://expense.example/sitemap.xml") {
		t.Fatalf("unexpected sitemap URL in robots: %q", body)
	}
}

func TestServeStaticFile_SitemapUsesConfiguredBaseURL(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://www.expenselog.com.ar")
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()

	h.ServeStaticFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/xml") {
		t.Fatalf("unexpected content-type: got %q", contentType)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<loc>https://www.expenselog.com.ar/</loc>") {
		t.Fatalf("unexpected sitemap XML: %q", body)
	}
}
