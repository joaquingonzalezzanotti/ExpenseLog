package api

import (
	"net/http/httptest"
	"testing"
)

func TestBeginMutationIdempotencyNoHeader(t *testing.T) {
	req := httptest.NewRequest("PUT", "/expense", nil)
	release, duplicate, err := beginMutationIdempotency(req, "scope-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duplicate {
		t.Fatalf("expected duplicate=false when header is absent")
	}
	release(true)
}

func TestBeginMutationIdempotencyRejectsInvalidKey(t *testing.T) {
	req := httptest.NewRequest("PUT", "/expense", nil)
	req.Header.Set("Idempotency-Key", "bad key with spaces")
	_, _, err := beginMutationIdempotency(req, "scope-b")
	if err == nil {
		t.Fatalf("expected invalid key error")
	}
}

func TestBeginMutationIdempotencyDetectsDuplicate(t *testing.T) {
	req1 := httptest.NewRequest("PUT", "/expense", nil)
	req1.Header.Set("Idempotency-Key", "dup-key-1")
	release1, duplicate1, err := beginMutationIdempotency(req1, "scope-c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duplicate1 {
		t.Fatalf("first request should not be duplicate")
	}
	release1(true)

	req2 := httptest.NewRequest("PUT", "/expense", nil)
	req2.Header.Set("Idempotency-Key", "dup-key-1")
	_, duplicate2, err := beginMutationIdempotency(req2, "scope-c")
	if err != nil {
		t.Fatalf("unexpected error on duplicate check: %v", err)
	}
	if !duplicate2 {
		t.Fatalf("second request should be duplicate")
	}
}

func TestBeginMutationIdempotencyRollbackOnFailure(t *testing.T) {
	req1 := httptest.NewRequest("PUT", "/expense", nil)
	req1.Header.Set("Idempotency-Key", "rollback-key-1")
	release1, duplicate1, err := beginMutationIdempotency(req1, "scope-d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duplicate1 {
		t.Fatalf("first request should not be duplicate")
	}
	release1(false)

	req2 := httptest.NewRequest("PUT", "/expense", nil)
	req2.Header.Set("Idempotency-Key", "rollback-key-1")
	release2, duplicate2, err := beginMutationIdempotency(req2, "scope-d")
	if err != nil {
		t.Fatalf("unexpected error after rollback: %v", err)
	}
	if duplicate2 {
		t.Fatalf("request after failed release should not be duplicate")
	}
	release2(true)
}
