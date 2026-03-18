package api

import (
	"strings"
	"sync"
	"time"
)

var emailDispatchGuardMu = &sync.Mutex{}
var emailDispatchGuardLastByKey = map[string]time.Time{}

func reserveEmailDispatch(key string, cooldown time.Duration, now time.Time) (func(success bool), bool) {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" || cooldown <= 0 {
		return func(bool) {}, false
	}

	emailDispatchGuardMu.Lock()
	if lastDispatch, ok := emailDispatchGuardLastByKey[normalizedKey]; ok {
		if now.Sub(lastDispatch) < cooldown {
			emailDispatchGuardMu.Unlock()
			return func(bool) {}, true
		}
	}
	emailDispatchGuardLastByKey[normalizedKey] = now
	emailDispatchGuardMu.Unlock()

	return func(success bool) {
		if success {
			return
		}
		emailDispatchGuardMu.Lock()
		defer emailDispatchGuardMu.Unlock()
		if lastDispatch, ok := emailDispatchGuardLastByKey[normalizedKey]; ok && lastDispatch.Equal(now) {
			delete(emailDispatchGuardLastByKey, normalizedKey)
		}
	}, false
}

func emailDispatchKey(kind, recipient string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "|" + strings.ToLower(strings.TrimSpace(recipient))
}
