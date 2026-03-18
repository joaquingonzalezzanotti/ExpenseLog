package api

import (
	"fmt"
	"testing"
	"time"
)

func TestReserveEmailDispatchCooldown(t *testing.T) {
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

