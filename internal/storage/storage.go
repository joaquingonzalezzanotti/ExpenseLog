package storage

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Storage interface for all storage types
type Storage interface {
	Close() error

	// Users
	CreateUser(email, passwordHash string) (User, error)
	CreateUserWithStatus(email, passwordHash, status string) (User, error)
	GetUserByEmail(email string) (User, error)
	GetUserByID(id string) (User, error)
	UpdateUserName(userID, name string) error
	UpdateUserPassword(userID, passwordHash string) error
	UpdateUserStatus(userID, status string) error
	CreateOAuthIdentity(identity OAuthIdentity) error
	GetOAuthIdentity(provider, providerUserID string) (OAuthIdentity, error)

	// Sessions
	CreateSession(session Session) error
	GetSession(id string) (Session, error)
	DeleteSession(id string) error
	DeleteSessionsByUserID(userID string) error

	// Password resets
	CreatePasswordReset(reset PasswordReset) error
	GetLatestPasswordReset(userID string) (PasswordReset, error)
	MarkPasswordResetUsed(resetID string) error
	RegisterPasswordResetFailure(resetID string) (attempts int, maxAttempts int, exhausted bool, err error)

	// Email verification
	CreateEmailVerification(verification EmailVerification) error
	GetEmailVerificationByTokenHash(tokenHash string) (EmailVerification, error)
	MarkEmailVerificationUsed(verificationID string) error

	// User Config
	GetConfig(userID string) (*Config, error)
	GetUserPlanTier(userID string) (string, error)

	// Basic Config Updates
	GetCategories(userID string) ([]string, error)
	UpdateCategories(userID string, categories []string) error
	// GetTags() ([]string, error)
	// UpdateTags(tags []string) error
	GetCurrency(userID string) (string, error)
	UpdateCurrency(userID string, currency string) error
	GetStartDate(userID string) (int, error)
	UpdateStartDate(userID string, startDate int) error

	// Recurring Expenses
	GetRecurringExpenses(userID string) ([]RecurringExpense, error)
	GetRecurringExpense(userID, id string) (RecurringExpense, error)
	AddRecurringExpense(userID string, recurringExpense RecurringExpense) error
	RemoveRecurringExpense(userID, id string, removeAll bool) error
	UpdateRecurringExpense(userID, id string, recurringExpense RecurringExpense, updateAll bool) error

	// Expenses
	GetAllExpenses(userID string) ([]Expense, error)
	GetExpensesByPeriodAndCurrency(userID string, start, end time.Time, currency string) ([]Expense, error)
	GetCashBalanceBeforeDate(userID string, before time.Time, currency string) (float64, error)
	GetExpense(userID, id string) (Expense, error)
	AddExpense(userID string, expense Expense) error
	RemoveExpense(userID, id string) error
	AddMultipleExpenses(userID string, expenses []Expense) error
	RemoveMultipleExpenses(userID string, ids []string) error
	UpdateExpense(userID, id string, expense Expense) error

	// Reconciliation
	ApplyReconciliation(userID string, input ReconciliationApplyInput) (ReconciliationApplyResult, error)
	GetReconciliationHistory(userID string) ([]ReconciliationRecord, error)
	RevertReconciliation(userID, adjustmentExpenseID string, now time.Time) (Expense, error)

	// Telegram Bot integration
	GetTelegramUserLinkByUserID(userID string) (TelegramUserLink, error)
	GetTelegramUserLinkByTelegramUserID(telegramUserID int64) (TelegramUserLink, error)
	GetActiveTelegramLinkCode(userID string, now time.Time) (TelegramLinkCode, error)
	InvalidateActiveTelegramLinkCodes(userID string, usedAt time.Time) error
	CreateTelegramLinkCode(userID, codeHash string, expiresAt, createdAt time.Time) (TelegramLinkCode, error)
	ConsumeTelegramLinkCode(codeHash string, telegramUserID int64, telegramUsername string, now time.Time) (TelegramUserLink, error)

	// Apple Wallet Shortcut ingestion
	GetActiveWalletIngestTokenByUserID(userID string) (WalletIngestToken, error)
	UpsertWalletIngestToken(userID, tokenHash string, now time.Time) (WalletIngestToken, error)
	GetWalletIngestTokenByHash(tokenHash string) (WalletIngestToken, error)
	TouchWalletIngestTokenLastUsed(tokenID string, usedAt time.Time) error
	CreateWalletIngestEvent(event WalletIngestEvent) (WalletIngestEvent, error)
	UpdateWalletIngestEventResult(eventID, status, confidence, createdTransactionID, duplicateOfEventID string) error
	FindPotentialDuplicateWalletIngestEvent(userID string, amount float64, merchantNormalized string, paidAt time.Time, window time.Duration) (WalletIngestEvent, error)

	// Potential Future Feature: Multi-currency
	// GetConversions() (map[string]float64, error)
	// UpdateConversions(conversions map[string]float64) error
}

var ErrSystemLockedExpense = errors.New("system-locked expense cannot be modified")
var ErrReconciliationNotFound = errors.New("reconciliation not found")
var ErrReconciliationAlreadyReverted = errors.New("reconciliation already reverted")
var ErrTelegramInvalidLinkCode = errors.New("invalid telegram link code")
var ErrTelegramLinkCodeExpired = errors.New("telegram link code expired")
var ErrTelegramLinkCodeUsed = errors.New("telegram link code already used")
var ErrTelegramPremiumRequired = errors.New("telegram premium required")
var ErrTelegramAlreadyLinked = errors.New("expenselog account already linked to another telegram user")
var ErrTelegramUserAlreadyLinked = errors.New("telegram user already linked to another expenselog account")

// config for expense data
type Config struct {
	Categories        []string           `json:"categories"`
	Currency          string             `json:"currency"`
	StartDate         int                `json:"startDate"`
	PlanTier          string             `json:"planTier"`
	RecurringExpenses []RecurringExpense `json:"recurringExpenses"`
	// Tags              []string           `json:"tags"`
}

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}

type PasswordReset struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	CodeHash    string    `json:"-"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CreatedAt   time.Time `json:"createdAt"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"maxAttempts"`
}

type EmailVerification struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	TokenHash  string    `json:"-"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
}

type OAuthIdentity struct {
	ID             string    `json:"id"`
	UserID         string    `json:"userId"`
	Provider       string    `json:"provider"`
	ProviderUserID string    `json:"providerUserId"`
	Email          string    `json:"email"`
	CreatedAt      time.Time `json:"createdAt"`
}

type RecurringExpense struct {
	UserID      string    `json:"-"`
	Flow        string    `json:"flow"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Tags        []string  `json:"tags"`
	Category    string    `json:"category"`
	StartDate   time.Time `json:"startDate"`   // date of the first occurrence
	Interval    string    `json:"interval"`    // daily, weekly, monthly, yearly
	Occurrences int       `json:"occurrences"` // 0 for 3000 occurrences (heuristic)
}

type BackendType string

const (
	// BackendTypeJSON is deprecated and no longer supported at runtime.
	BackendTypeJSON     BackendType = "json"
	BackendTypePostgres BackendType = "postgres"
)

// config for the storage backend
type SystemConfig struct {
	StorageURL  string
	StorageType BackendType
	StorageUser string
	StoragePass string
	StorageSSL  string
}

// expense struct
type Expense struct {
	UserID       string    `json:"-"`
	Flow         string    `json:"flow"`
	ID           string    `json:"id"`
	RecurringID  string    `json:"recurringID"`
	Name         string    `json:"name"`
	Tags         []string  `json:"tags"`
	Category     string    `json:"category"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	Source       string    `json:"source"`
	Card         string    `json:"card"`
	SystemOrigin string    `json:"systemOrigin,omitempty"`
	SystemLocked bool      `json:"systemLocked,omitempty"`
	Date         time.Time `json:"date"`
}

type WalletIngestEvent struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"userId"`
	Source               string    `json:"source"`
	Amount               float64   `json:"amount"`
	Merchant             string    `json:"merchant"`
	MerchantRaw          string    `json:"merchantRaw"`
	CardLabel            string    `json:"cardLabel"`
	WalletCategory       string    `json:"walletCategory"`
	PaidAt               time.Time `json:"paidAt"`
	RawPayload           string    `json:"rawPayload"`
	RequestHeaders       string    `json:"requestHeaders,omitempty"`
	Status               string    `json:"status"`
	Confidence           string    `json:"confidence"`
	CreatedTransactionID string    `json:"createdTransactionId"`
	DuplicateOfEventID   string    `json:"duplicateOfEventId"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type WalletIngestToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	TokenHash  string     `json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type ReconciliationApplyInput struct {
	TargetBalance  float64
	Currency       string
	Note           string
	IdempotencyKey string
	Now            time.Time
}

type ReconciliationApplyResult struct {
	Status         string
	Expense        Expense
	CurrentBalance float64
	Difference     float64
	Currency       string
}

type ReconciliationRecord struct {
	ID                  string
	UserID              string
	AdjustmentExpenseID string
	ReversalExpenseID   string
	TargetBalance       *float64
	AppBalanceBefore    *float64
	DeltaAmount         float64
	Currency            string
	Note                string
	IdempotencyKey      string
	Status              string
	CreatedAt           time.Time
	RevertedAt          *time.Time
}

type TelegramUserLink struct {
	ID               string
	UserID           string
	TelegramUserID   int64
	TelegramUsername string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TelegramLinkCode struct {
	ID                   string
	UserID               string
	CodeHash             string
	CreatedAt            time.Time
	ExpiresAt            time.Time
	UsedAt               *time.Time
	UsedByTelegramUserID *int64
	UsedTelegramUsername string
}

func (c *Config) SetBaseConfig() {
	c.Categories = defaultCategories
	c.Currency = "ars"
	c.StartDate = 1
	c.PlanTier = PlanTierFree
	// c.Tags = []string{}
	c.RecurringExpenses = []RecurringExpense{}
}

const (
	PlanTierFree    = "free"
	PlanTierPremium = "premium"
)

func NormalizePlanTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PlanTierPremium:
		return PlanTierPremium
	default:
		return PlanTierFree
	}
}

func (c *SystemConfig) SetStorageConfig() {
	c.StorageType = backendTypeFromEnv(os.Getenv("STORAGE_TYPE"))
	c.StorageURL = backendURLFromEnv(os.Getenv("STORAGE_URL"))
	c.StorageSSL = backendSSLFromEnv(os.Getenv("STORAGE_SSL"))
	c.StorageUser = os.Getenv("STORAGE_USER")
	c.StoragePass = os.Getenv("STORAGE_PASS")
}

func backendTypeFromEnv(env string) BackendType {
	switch env {
	case "json":
		return BackendTypeJSON
	case "postgres":
		return BackendTypePostgres
	default:
		return ""
	}
}

func backendURLFromEnv(env string) string {
	return env
}

func backendSSLFromEnv(env string) string {
	switch env {
	case "disable", "require", "verify-full", "verify-ca":
		return env
	default:
		return "require"
	}
}

// initializes the storage backend
func InitializeStorage() (Storage, error) {
	baseConfig := SystemConfig{}
	baseConfig.SetStorageConfig()
	if baseConfig.StorageType == "" {
		return nil, fmt.Errorf("missing STORAGE_TYPE (set STORAGE_TYPE=postgres)")
	}
	if baseConfig.StorageType != BackendTypePostgres {
		return nil, fmt.Errorf("unsupported storage type: %q (json storage deprecated; set STORAGE_TYPE=postgres)", baseConfig.StorageType)
	}
	if baseConfig.StorageURL == "" {
		return nil, fmt.Errorf("missing STORAGE_URL for postgres backend")
	}
	if baseConfig.StorageUser == "" {
		return nil, fmt.Errorf("missing STORAGE_USER for postgres backend")
	}
	if baseConfig.StoragePass == "" {
		return nil, fmt.Errorf("missing STORAGE_PASS for postgres backend")
	}
	return InitializePostgresStore(baseConfig)
}

var REInvalidChars *regexp.Regexp = regexp.MustCompile(`[^\p{L}\p{N}\s.,\-'_!"]`)
var RERepeatingSpaces *regexp.Regexp = regexp.MustCompile(`\s+`)

// allows readable chars like unicode, otherwise replaces with whitespace
func SanitizeString(s string) string {
	sanitized := REInvalidChars.ReplaceAllString(s, " ")
	sanitized = RERepeatingSpaces.ReplaceAllString(sanitized, " ")
	return strings.TrimSpace(sanitized)
}

func ValidateCategory(category string) (string, error) {
	sanitized := SanitizeString(category)
	if sanitized == "" {
		return "", fmt.Errorf("category name cannot be empty or contain only invalid characters")
	}
	return sanitized, nil
}

func ValidateUserName(name string) (string, error) {
	sanitized := SanitizeString(name)
	if sanitized == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	if len([]rune(sanitized)) > 80 {
		return "", fmt.Errorf("name cannot exceed 80 characters")
	}
	return sanitized, nil
}

func (e *Expense) Validate() error {
	e.Name = SanitizeString(e.Name)
	if e.Name == "" {
		return fmt.Errorf("expense 'name' cannot be empty")
	}
	if e.Category == "" {
		return fmt.Errorf("expense 'category' cannot be empty")
	}
	if e.Amount == 0 {
		return fmt.Errorf("expense 'amount' cannot be 0")
	}
	e.Source = SanitizeString(e.Source)
	e.Card = SanitizeString(e.Card)
	// if e.Currency == "" {
	// 	return fmt.Errorf("expense 'currency' cannot be empty")
	// }
	if len(e.Tags) > 0 {
		var cleanedTags []string
		for _, tag := range e.Tags {
			sanitizedTag := SanitizeString(tag)
			if sanitizedTag != "" {
				cleanedTags = append(cleanedTags, sanitizedTag)
			}
		}
		e.Tags = cleanedTags
	}
	if e.Date.IsZero() {
		return fmt.Errorf("expense 'date' cannot be empty")
	}
	return nil
}

func (e *RecurringExpense) Validate() error {
	e.Name = SanitizeString(e.Name)
	if e.Name == "" {
		return fmt.Errorf("recurring expense 'name' cannot be empty")
	}
	if e.Category == "" {
		return fmt.Errorf("recurring expense 'category' cannot be empty")
	}
	if len(e.Tags) > 0 {
		var cleanedTags []string
		for _, tag := range e.Tags {
			sanitizedTag := SanitizeString(tag)
			if sanitizedTag != "" {
				cleanedTags = append(cleanedTags, sanitizedTag)
			}
		}
		e.Tags = cleanedTags
	}
	if e.Occurrences < 2 {
		return fmt.Errorf("at least 2 occurences required to recur")
	}
	if e.StartDate.IsZero() {
		return fmt.Errorf("start date for recurring expense must be specified")
	}
	validIntervals := map[string]bool{
		"daily":   true,
		"weekly":  true,
		"monthly": true,
		"yearly":  true,
	}
	if !validIntervals[e.Interval] {
		return fmt.Errorf("invalid interval: '%s'. Must be one of 'daily', 'weekly', 'monthly', or 'yearly'", e.Interval)
	}
	return nil
}

// variables
var defaultCategories = []string{
	"Comida",
	"Supermercado",
	"Viajes",
	"Alquiler",
	"Servicios",
	"Entretenimiento",
	"Salud",
	"Compras",
	"Varios",
	"Ingresos",
}

var SupportedCurrencies = []string{
	"ars", // Argentine Peso
	"usd", // US Dollar
	"eur", // Euro
}
