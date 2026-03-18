package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const mutationIdempotencyTTL = 3 * time.Minute

var mutationIdempotencyMu = &sync.Mutex{}
var mutationIdempotencyState = map[string]time.Time{}

func beginMutationIdempotency(r *http.Request, scope string) (release func(success bool), duplicate bool, err error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return func(bool) {}, false, nil
	}
	if !isValidIdempotencyKey(key) {
		return nil, false, fmt.Errorf("invalid Idempotency-Key")
	}

	now := time.Now()
	composite := strings.ToLower(strings.TrimSpace(scope)) + "|" + strings.ToLower(key)

	mutationIdempotencyMu.Lock()
	for stateKey, expiresAt := range mutationIdempotencyState {
		if now.After(expiresAt) {
			delete(mutationIdempotencyState, stateKey)
		}
	}
	if expiresAt, ok := mutationIdempotencyState[composite]; ok && now.Before(expiresAt) {
		mutationIdempotencyMu.Unlock()
		return func(bool) {}, true, nil
	}
	mutationIdempotencyState[composite] = now.Add(mutationIdempotencyTTL)
	mutationIdempotencyMu.Unlock()

	return func(success bool) {
		if success {
			return
		}
		mutationIdempotencyMu.Lock()
		defer mutationIdempotencyMu.Unlock()
		delete(mutationIdempotencyState, composite)
	}, false, nil
}

func isValidIdempotencyKey(key string) bool {
	if len(key) > 120 {
		return false
	}
	for _, ch := range key {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}
