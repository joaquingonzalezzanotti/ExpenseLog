package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterOperationalEndpointsHealthAndReady(t *testing.T) {
	mux := http.NewServeMux()
	obs := newObservabilityRegistry()
	registerOperationalEndpoints(mux, "test-version", obs, func(ctx context.Context) error {
		return nil
	})

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	mux.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", healthRec.Code)
	}
	var healthPayload map[string]any
	if err := json.Unmarshal(healthRec.Body.Bytes(), &healthPayload); err != nil {
		t.Fatalf("health payload invalid json: %v", err)
	}
	if healthPayload["status"] != "ok" {
		t.Fatalf("expected health status ok, got %v", healthPayload["status"])
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readyRec := httptest.NewRecorder()
	mux.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("expected ready status 200, got %d", readyRec.Code)
	}
	var readyPayload map[string]any
	if err := json.Unmarshal(readyRec.Body.Bytes(), &readyPayload); err != nil {
		t.Fatalf("ready payload invalid json: %v", err)
	}
	if readyPayload["status"] != "ready" {
		t.Fatalf("expected ready status ready, got %v", readyPayload["status"])
	}
}

func TestRegisterOperationalEndpointsReadyDegraded(t *testing.T) {
	mux := http.NewServeMux()
	obs := newObservabilityRegistry()
	registerOperationalEndpoints(mux, "test-version", obs, func(ctx context.Context) error {
		return errors.New("db unavailable")
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected ready status 503, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("ready degraded payload invalid json: %v", err)
	}
	if payload["status"] != "degraded" {
		t.Fatalf("expected degraded status, got %v", payload["status"])
	}
}

func TestWithRequestIDSetsHeaderAndContext(t *testing.T) {
	handler := withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Debug-Request-ID", requestIDFromContext(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestIDHeader, "request-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get(requestIDHeader); got != "request-123" {
		t.Fatalf("expected response request id request-123, got %q", got)
	}
	if got := rec.Header().Get("X-Debug-Request-ID"); got != "request-123" {
		t.Fatalf("expected context request id request-123, got %q", got)
	}
}

func TestWithAccessLoggingAndMetricsRecordsRequest(t *testing.T) {
	obs := newObservabilityRegistry()
	handler := withAccessLoggingAndMetrics(obs, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/expense", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	snapshot := obs.snapshot()
	route, ok := snapshot.Routes["POST /expense"]
	if !ok {
		t.Fatalf("expected POST /expense route metrics")
	}
	if route.Requests != 1 {
		t.Fatalf("expected 1 request for route, got %d", route.Requests)
	}
}
