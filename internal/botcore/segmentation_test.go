package botcore

import "testing"

func TestSplitBatchTextCandidatesByNewline(t *testing.T) {
	candidates := SplitBatchTextCandidates("350000 en super\n15000 en kiosko\n150000 ingreso")
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestSplitBatchTextCandidatesBySemicolon(t *testing.T) {
	candidates := SplitBatchTextCandidates("350000 en super; 15000 en kiosko; 150000 ingreso")
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestSplitBatchTextCandidatesSingle(t *testing.T) {
	candidates := SplitBatchTextCandidates("gaste 2000 en super")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}
