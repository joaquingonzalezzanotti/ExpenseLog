package api

import (
	"fmt"
	"testing"
	"time"
)

func resetEmailDispatchGuardState() {
	emailDispatchGuardMu.Lock()
	defer emailDispatchGuardMu.Unlock()
	emailDispatchGuardLastByKey = map[string]time.Time{}
}

func TestReserveEmailDispatchCooldown(t *testing.T) {
	resetEmailDispatchGuardState()
	key := fmt.Sprintf("test-cooldown-%d", time.Now().UnixNano())
	now := time.Now()
	cooldown := 30 * time.Second

	releaseFirst, skipFirst := reserveEmailDispatch(key, cooldown, now)
	if skipFirst {
		t.Fatalf("first dispatch should not be skipped")
	}
	releaseFirst(true)

	_, skipSecond := reserveEmailDispatch(key, cooldown, now.Add(5*time.Second))
	if !skipSecond {
		t.Fatalf("second dispatch inside cooldown should be skipped")
	}
}

func TestReserveEmailDispatchRollbackOnFailure(t *testing.T) {
	resetEmailDispatchGuardState()
	key := fmt.Sprintf("test-rollback-%d", time.Now().UnixNano())
	now := time.Now()
	cooldown := 30 * time.Second

	releaseFirst, skipFirst := reserveEmailDispatch(key, cooldown, now)
	if skipFirst {
		t.Fatalf("first dispatch should not be skipped")
	}
	releaseFirst(false)

	_, skipSecond := reserveEmailDispatch(key, cooldown, now.Add(5*time.Second))
	if skipSecond {
		t.Fatalf("dispatch after failed send should not be skipped")
	}
}

func TestReserveEmailDispatchPurgesStaleKeys(t *testing.T) {
	resetEmailDispatchGuardState()

	staleNow := time.Now().Add(-(emailDispatchGuardRetention + time.Hour))
	staleKey := "stale-key"
	activeKey := "active-key"
	emailDispatchGuardLastByKey[staleKey] = staleNow
	emailDispatchGuardLastByKey[activeKey] = time.Now()

	_, _ = reserveEmailDispatch("fresh-key", 30*time.Second, time.Now())

	emailDispatchGuardMu.Lock()
	defer emailDispatchGuardMu.Unlock()
	if _, ok := emailDispatchGuardLastByKey[staleKey]; ok {
		t.Fatalf("expected stale key to be removed")
	}
	if _, ok := emailDispatchGuardLastByKey[activeKey]; !ok {
		t.Fatalf("expected active key to remain")
	}
}
