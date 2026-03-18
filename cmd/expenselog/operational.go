package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const requestIDHeader = "X-Request-ID"

type contextKey string

const requestIDContextKey contextKey = "requestID"

type readyChecker func(ctx context.Context) error

type routeObservability struct {
	Requests      int64         `json:"requests"`
	Errors5xx     int64         `json:"errors5xx"`
	TotalDuration time.Duration `json:"-"`
	MaxDuration   time.Duration `json:"-"`
}

type observabilityRegistry struct {
	mu       sync.Mutex
	started  time.Time
	byRoute  map[string]*routeObservability
	totalReq int64
}

type routeObservabilitySnapshot struct {
	Requests      int64   `json:"requests"`
	Errors5xx     int64   `json:"errors5xx"`
	AvgLatencyMS  float64 `json:"avgLatencyMs"`
	MaxLatencyMS  float64 `json:"maxLatencyMs"`
	ErrorRatePerc float64 `json:"errorRatePerc"`
}

type metricsSnapshot struct {
	GeneratedAt string                                `json:"generatedAt"`
	UptimeSec   int64                                 `json:"uptimeSec"`
	TotalReq    int64                                 `json:"totalRequests"`
	Routes      map[string]routeObservabilitySnapshot `json:"routes"`
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func newObservabilityRegistry() *observabilityRegistry {
	return &observabilityRegistry{
		started: time.Now(),
		byRoute: map[string]*routeObservability{},
	}
}

func (o *observabilityRegistry) record(method, path string, status int, duration time.Duration) {
	key := method + " " + path
	o.mu.Lock()
	defer o.mu.Unlock()
	entry, ok := o.byRoute[key]
	if !ok {
		entry = &routeObservability{}
		o.byRoute[key] = entry
	}
	entry.Requests++
	if status >= 500 {
		entry.Errors5xx++
	}
	entry.TotalDuration += duration
	if duration > entry.MaxDuration {
		entry.MaxDuration = duration
	}
	o.totalReq++
}

func (o *observabilityRegistry) snapshot() metricsSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()

	routes := make(map[string]routeObservabilitySnapshot, len(o.byRoute))
	for key, value := range o.byRoute {
		avgMs := 0.0
		if value.Requests > 0 {
			avgMs = float64(value.TotalDuration.Milliseconds()) / float64(value.Requests)
		}
		errorRate := 0.0
		if value.Requests > 0 {
			errorRate = (float64(value.Errors5xx) / float64(value.Requests)) * 100
		}
		routes[key] = routeObservabilitySnapshot{
			Requests:      value.Requests,
			Errors5xx:     value.Errors5xx,
			AvgLatencyMS:  avgMs,
			MaxLatencyMS:  float64(value.MaxDuration.Milliseconds()),
			ErrorRatePerc: errorRate,
		}
	}

	return metricsSnapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UptimeSec:   int64(time.Since(o.started).Seconds()),
		TotalReq:    o.totalReq,
		Routes:      routes,
	}
}

func registerOperationalEndpoints(mux *http.ServeMux, version string, obs *observabilityRegistry, checkReady readyChecker) {
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "expenselog",
			"version": version,
			"time":    time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/health", healthHandler)

	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := checkReady(ctx); err != nil {
			writeJSONResponse(w, http.StatusServiceUnavailable, map[string]any{
				"status":  "degraded",
				"service": "expenselog",
				"error":   "storage_unavailable",
			})
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"status":  "ready",
			"service": "expenselog",
		})
	}
	mux.HandleFunc("/ready", readyHandler)
	mux.HandleFunc("/api/ready", readyHandler)

	metricsHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSONResponse(w, http.StatusOK, obs.snapshot())
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/api/metrics", metricsHandler)
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitizeRequestID(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set(requestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf(`{"level":"error","event":"panic","requestId":"%s","path":"%s","method":"%s","error":"%v"}`,
					requestIDFromContext(r.Context()),
					r.URL.Path,
					r.Method,
					recovered,
				)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withAccessLoggingAndMetrics(obs *observabilityRegistry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		duration := time.Since(start)
		obs.record(r.Method, r.URL.Path, recorder.status, duration)
		logAccess(r, recorder.status, duration)
	})
}

func logAccess(r *http.Request, status int, duration time.Duration) {
	remoteIP := readRemoteIP(r.RemoteAddr)
	payload := map[string]any{
		"event":       "http_request",
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
		"requestId":   requestIDFromContext(r.Context()),
		"method":      r.Method,
		"path":        r.URL.Path,
		"status":      status,
		"durationMs":  duration.Milliseconds(),
		"remoteIp":    remoteIP,
		"userAgent":   r.UserAgent(),
		"queryString": r.URL.RawQuery,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("request log marshal failed: %v", err)
		return
	}
	log.Printf("%s", string(raw))
}

func writeJSONResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func readRemoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 120 {
		value = value[:120]
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.') {
			return ""
		}
	}
	return value
}

func generateRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconvTimestampID()
	}
	return hex.EncodeToString(buf)
}

func strconvTimestampID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}
