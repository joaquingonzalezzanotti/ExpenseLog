package botcore

import "testing"

func TestDecideDefaults(t *testing.T) {
	decision, err := Decide(DecisionRequest{
		Candidate: ParseCandidate{
			Channel:  ChannelWhatsApp,
			Amount:   1500,
			FlowHint: "",
		},
		DefaultCurrency: "ars",
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Flow != "income" {
		t.Fatalf("expected income flow, got %s", decision.Flow)
	}
	if decision.Amount != 1500 {
		t.Fatalf("expected positive amount, got %.2f", decision.Amount)
	}
	if decision.Currency != "ars" {
		t.Fatalf("expected ars currency, got %s", decision.Currency)
	}
}

func TestDecideMarksAmbiguousOnAliasConflict(t *testing.T) {
	decision, err := Decide(DecisionRequest{
		Candidate: ParseCandidate{
			Channel:      ChannelTelegram,
			Amount:       30000,
			FlowHint:     "expense",
			Counterparty: "Joaquin Gonzalez",
		},
		Identity: IdentityProfile{
			Aliases: []IdentityAlias{
				{AliasNorm: "joaquin gonzalez", Confidence: 1},
			},
		},
		DefaultCurrency: "ars",
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if !decision.Ambiguous {
		t.Fatalf("expected ambiguous decision when flow conflicts with alias ownership")
	}
	if decision.Flow != "expense" {
		t.Fatalf("flow should stay as hinted on conflict, got %s", decision.Flow)
	}
}

func TestDecideUsesAccountOwnershipSignalWhenFlowMissing(t *testing.T) {
	decision, err := Decide(DecisionRequest{
		Candidate: ParseCandidate{
			Channel:  ChannelTelegram,
			Amount:   100000,
			FlowHint: "",
			Evidence: ParseEvidence{
				DestinationAccountLast4: "8536",
			},
		},
		Identity: IdentityProfile{
			OwnedAccounts: []OwnedAccountFingerprint{
				{AccountLast4: "8536", Confidence: 1},
			},
		},
		DefaultCurrency: "ars",
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Flow != "income" {
		t.Fatalf("expected income flow due to destination ownership, got %s", decision.Flow)
	}
	if decision.Amount <= 0 {
		t.Fatalf("expected positive amount for income, got %.2f", decision.Amount)
	}
}
