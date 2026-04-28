package botcore

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

type Channel string

const (
	ChannelTelegram Channel = "telegram"
	ChannelWhatsApp Channel = "whatsapp"
)

type ParseEvidence struct {
	RawText                 string
	MediaCaption            string
	SourceAccountLast4      string
	DestinationAccountLast4 string
}

type ParseCandidate struct {
	Channel      Channel
	FlowHint     string
	Amount       float64
	Currency     string
	DateTime     time.Time
	Counterparty string
	Reference    string
	Motive       string
	CategoryHint string
	SourceHint   string
	Tags         []string
	Evidence     ParseEvidence
}

type IdentityAlias struct {
	AliasNorm  string
	Confidence float64
}

type OwnedAccountFingerprint struct {
	AccountLast4 string
	CBUCVULast4  string
	Confidence   float64
}

type IdentityProfile struct {
	Aliases       []IdentityAlias
	OwnedAccounts []OwnedAccountFingerprint
}

type DecisionRequest struct {
	Candidate       ParseCandidate
	Identity        IdentityProfile
	DefaultCurrency string
	Now             time.Time
}

type Decision struct {
	Flow       string
	Amount     float64
	Currency   string
	Category   string
	Source     string
	Name       string
	DateTime   time.Time
	Confidence float64
	Ambiguous  bool
	Reasons    []string
}

func Decide(req DecisionRequest) (Decision, error) {
	flow, amount, flowProvided, err := normalizeFlow(req.Candidate.FlowHint, req.Candidate.Amount)
	if err != nil {
		return Decision{}, err
	}

	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	currency := strings.ToLower(strings.TrimSpace(req.Candidate.Currency))
	if currency == "" {
		currency = strings.ToLower(strings.TrimSpace(req.DefaultCurrency))
	}
	if currency == "" {
		currency = "ars"
	}

	name := buildMovementName(req.Candidate.Channel, req.Candidate.Counterparty, req.Candidate.Motive, req.Candidate.Reference)
	category := resolveCategory(flow, req.Candidate.CategoryHint)
	source := normalizeSource(req.Candidate.SourceHint)
	when := req.Candidate.DateTime.UTC()
	if when.IsZero() {
		when = now
	}

	decision := Decision{
		Flow:       flow,
		Amount:     amount,
		Currency:   currency,
		Category:   category,
		Source:     source,
		Name:       name,
		DateTime:   when,
		Confidence: baseConfidence(flowProvided),
		Reasons:    []string{"flow_normalized"},
	}

	applyIdentitySignals(&decision, req.Candidate, req.Identity, flowProvided)
	decision.Confidence = clamp01(decision.Confidence)
	return decision, nil
}

func normalizeFlow(raw string, amount float64) (flow string, signedAmount float64, provided bool, err error) {
	absAmount := math.Abs(amount)
	if absAmount == 0 {
		return "", 0, false, fmt.Errorf("amount must be non-zero")
	}
	flow = strings.ToLower(strings.TrimSpace(raw))
	if flow == "" {
		provided = false
		if amount > 0 {
			flow = "income"
		} else {
			flow = "expense"
		}
	} else {
		provided = true
	}
	switch flow {
	case "income", "refund":
		return flow, absAmount, provided, nil
	case "expense":
		return flow, -absAmount, provided, nil
	default:
		return "", 0, provided, fmt.Errorf("invalid flow: %s", flow)
	}
}

func baseConfidence(flowProvided bool) float64 {
	if flowProvided {
		return 0.88
	}
	return 0.72
}

func resolveCategory(flow string, rawCategory string) string {
	category := storage.SanitizeString(strings.TrimSpace(rawCategory))
	if category != "" {
		return category
	}
	if flow == "income" || flow == "refund" {
		return "Ingresos"
	}
	return "Varios"
}

func normalizeSource(raw string) string {
	clean := strings.ToUpper(storage.SanitizeString(raw))
	switch {
	case clean == "":
		return "CA"
	case strings.Contains(clean, "EFECTIVO"), strings.Contains(clean, "CASH"):
		return "EFECTIVO"
	case strings.Contains(clean, "DEBITO"),
		strings.Contains(clean, "DEBIT"),
		strings.Contains(clean, "TRANSFER"),
		strings.Contains(clean, "BANCO"),
		strings.Contains(clean, "BANK"),
		strings.Contains(clean, "WALLET"),
		strings.Contains(clean, "MODO"),
		strings.Contains(clean, "GALICIA"):
		return "CA"
	case strings.Contains(clean, "TARJETA"),
		strings.Contains(clean, "CREDITO"),
		strings.Contains(clean, "CREDIT"),
		strings.Contains(clean, "VISA"),
		strings.Contains(clean, "MASTERCARD"),
		strings.Contains(clean, "AMEX"):
		return "TARJETA"
	default:
		return "CA"
	}
}

func buildMovementName(channel Channel, counterparty, motive, reference string) string {
	base := storage.SanitizeString(strings.TrimSpace(counterparty))
	if base == "" {
		switch channel {
		case ChannelWhatsApp:
			base = "Movimiento WhatsApp"
		default:
			base = "Movimiento Telegram"
		}
	}
	motive = storage.SanitizeString(strings.TrimSpace(motive))
	reference = storage.SanitizeString(strings.TrimSpace(reference))
	switch {
	case motive != "" && reference != "":
		return fmt.Sprintf("%s - %s (%s)", base, motive, reference)
	case motive != "":
		return fmt.Sprintf("%s - %s", base, motive)
	case reference != "":
		return fmt.Sprintf("%s (%s)", base, reference)
	default:
		return base
	}
}

func applyIdentitySignals(decision *Decision, candidate ParseCandidate, identity IdentityProfile, flowProvided bool) {
	if decision == nil {
		return
	}
	counterparty := strings.ToLower(storage.SanitizeString(candidate.Counterparty))
	aliasMatch := false
	for _, alias := range identity.Aliases {
		norm := strings.ToLower(strings.TrimSpace(alias.AliasNorm))
		if norm == "" {
			continue
		}
		if counterparty == norm || strings.Contains(counterparty, norm) {
			aliasMatch = true
			break
		}
	}
	if aliasMatch {
		decision.Reasons = append(decision.Reasons, "counterparty_matches_identity_alias")
		if !flowProvided {
			decision.Flow = "income"
			decision.Amount = math.Abs(decision.Amount)
			decision.Confidence += 0.08
			return
		}
		if decision.Flow == "expense" {
			decision.Ambiguous = true
			decision.Confidence -= 0.32
			decision.Reasons = append(decision.Reasons, "flow_hint_conflicts_with_identity_alias")
		}
	}

	destOwn := accountLast4MatchesIdentity(candidate.Evidence.DestinationAccountLast4, identity)
	srcOwn := accountLast4MatchesIdentity(candidate.Evidence.SourceAccountLast4, identity)
	switch {
	case destOwn && !srcOwn:
		decision.Reasons = append(decision.Reasons, "destination_account_matches_identity")
		if !flowProvided {
			decision.Flow = "income"
			decision.Amount = math.Abs(decision.Amount)
			decision.Confidence += 0.1
		} else if decision.Flow == "expense" {
			decision.Ambiguous = true
			decision.Confidence -= 0.35
			decision.Reasons = append(decision.Reasons, "flow_hint_conflicts_with_destination_ownership")
		}
	case srcOwn && !destOwn:
		decision.Reasons = append(decision.Reasons, "source_account_matches_identity")
		if !flowProvided {
			decision.Flow = "expense"
			decision.Amount = -math.Abs(decision.Amount)
			decision.Confidence += 0.1
		} else if decision.Flow == "income" {
			decision.Ambiguous = true
			decision.Confidence -= 0.35
			decision.Reasons = append(decision.Reasons, "flow_hint_conflicts_with_source_ownership")
		}
	}
}

func accountLast4MatchesIdentity(rawLast4 string, identity IdentityProfile) bool {
	last4 := normalizeLast4(rawLast4)
	if last4 == "" {
		return false
	}
	for _, fp := range identity.OwnedAccounts {
		if normalizeLast4(fp.AccountLast4) == last4 || normalizeLast4(fp.CBUCVULast4) == last4 {
			return true
		}
	}
	return false
}

func normalizeLast4(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return ""
	}
	if len(digits) <= 4 {
		return digits
	}
	return digits[len(digits)-4:]
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
