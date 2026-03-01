package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// databaseStore implements the Storage interface for PostgreSQL.
type databaseStore struct {
	db *sql.DB
}

// SQL queries as constants for reusability and clarity.
const (
	createUsersTableSQL = `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(36) PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	createSessionsTableSQL = `
	CREATE TABLE IF NOT EXISTS sessions (
		id VARCHAR(64) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at TIMESTAMPTZ NOT NULL,
		ip VARCHAR(100),
		user_agent TEXT
	);`

	createPasswordResetsTableSQL = `
	CREATE TABLE IF NOT EXISTS password_resets (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		code_hash TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 5,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at TIMESTAMPTZ NOT NULL,
		used_at TIMESTAMPTZ
	);`

	createEmailVerificationsTableSQL = `
	CREATE TABLE IF NOT EXISTS email_verifications (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		token_hash TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at TIMESTAMPTZ NOT NULL,
		verified_at TIMESTAMPTZ
	);`

	createOAuthIdentitiesTableSQL = `
	CREATE TABLE IF NOT EXISTS oauth_identities (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		provider VARCHAR(50) NOT NULL,
		provider_user_id TEXT NOT NULL,
		email TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	createUserConfigTableSQL = `
	CREATE TABLE IF NOT EXISTS user_config (
		user_id VARCHAR(36) PRIMARY KEY,
		currency VARCHAR(255) NOT NULL,
		start_date INTEGER NOT NULL,
		plan_tier VARCHAR(20) NOT NULL DEFAULT 'free'
	);`

	createExpensesTableSQL = `
	CREATE TABLE IF NOT EXISTS expenses (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		recurring_id VARCHAR(36),
		name VARCHAR(255) NOT NULL,
		category VARCHAR(255) NOT NULL,
		amount NUMERIC(10, 2) NOT NULL,
		currency VARCHAR(3) NOT NULL,
		date TIMESTAMPTZ NOT NULL,
		flow VARCHAR(20) NOT NULL DEFAULT 'expense',
		tags TEXT,
		source VARCHAR(50),
		card VARCHAR(100),
		system_origin VARCHAR(50) NOT NULL DEFAULT 'user',
		system_locked BOOLEAN NOT NULL DEFAULT FALSE
	);`

	createReconciliationsTableSQL = `
	CREATE TABLE IF NOT EXISTS reconciliations (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		adjustment_expense_id VARCHAR(36) NOT NULL,
		reversal_expense_id VARCHAR(36),
		target_balance NUMERIC(14, 2),
		app_balance_before NUMERIC(14, 2),
		delta_amount NUMERIC(14, 2) NOT NULL,
		currency VARCHAR(3) NOT NULL,
		note TEXT,
		idempotency_key VARCHAR(120),
		status VARCHAR(20) NOT NULL DEFAULT 'applied',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		reverted_at TIMESTAMPTZ
	);`

	createRecurringExpensesTableSQL = `
	CREATE TABLE IF NOT EXISTS recurring_expenses (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		amount NUMERIC(10, 2) NOT NULL,
		currency VARCHAR(3) NOT NULL,
		category VARCHAR(255) NOT NULL,
		start_date TIMESTAMPTZ NOT NULL,
		interval VARCHAR(50) NOT NULL,
		occurrences INTEGER NOT NULL,
		flow VARCHAR(20) NOT NULL DEFAULT 'expense',
		tags TEXT
	);`

	createConfigTableSQL = `
	CREATE TABLE IF NOT EXISTS config (
		id VARCHAR(255) PRIMARY KEY DEFAULT 'default',
		categories TEXT NOT NULL,
		currency VARCHAR(255) NOT NULL,
		start_date INTEGER NOT NULL
	);`

	createCategoriesTableSQL = `
	CREATE TABLE IF NOT EXISTS categories (
		id SERIAL PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		name TEXT NOT NULL,
		position INTEGER NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	createTelegramUserLinksTableSQL = `
	CREATE TABLE IF NOT EXISTS telegram_user_links (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		telegram_user_id BIGINT NOT NULL,
		telegram_username TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	createTelegramLinkCodesTableSQL = `
	CREATE TABLE IF NOT EXISTS telegram_link_codes (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		code_hash VARCHAR(128) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at TIMESTAMPTZ NOT NULL,
		used_at TIMESTAMPTZ,
		used_by_telegram_user_id BIGINT,
		used_telegram_username TEXT
	);`
)

func InitializePostgresStore(baseConfig SystemConfig) (Storage, error) {
	dbURL := makeDBURL(baseConfig)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL database: %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %v", err)
	}
	log.Println("Connected to PostgreSQL database")

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("failed to create database tables: %v", err)
	}
	if err := ensureBootstrapUser(db); err != nil {
		return nil, fmt.Errorf("failed to bootstrap user data: %v", err)
	}
	return &databaseStore{db: db}, nil
}

func makeDBURL(baseConfig SystemConfig) string {
	options := []string{fmt.Sprintf("sslmode=%s", baseConfig.StorageSSL)}
	if strings.Contains(baseConfig.StorageURL, "pooler") || strings.EqualFold(os.Getenv("STORAGE_PREFER_SIMPLE_PROTOCOL"), "true") {
		options = append(options, "prefer_simple_protocol=true")
	}
	return fmt.Sprintf("postgres://%s:%s@%s?%s", baseConfig.StorageUser, baseConfig.StoragePass, baseConfig.StorageURL, strings.Join(options, "&"))
}

func createTables(db *sql.DB) error {
	for _, query := range []string{
		createUsersTableSQL,
		createSessionsTableSQL,
		createPasswordResetsTableSQL,
		createEmailVerificationsTableSQL,
		createOAuthIdentitiesTableSQL,
		createUserConfigTableSQL,
		createExpensesTableSQL,
		createReconciliationsTableSQL,
		createRecurringExpensesTableSQL,
		createConfigTableSQL,
		createCategoriesTableSQL,
		createTelegramUserLinksTableSQL,
		createTelegramLinkCodesTableSQL,
	} {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	// ensure columns exist for backward compatibility
	alterStmts := []string{
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS user_id VARCHAR(36)",
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS source VARCHAR(50)",
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS card VARCHAR(100)",
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS flow VARCHAR(20) NOT NULL DEFAULT 'expense'",
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS system_origin VARCHAR(50) NOT NULL DEFAULT 'user'",
		"ALTER TABLE expenses ADD COLUMN IF NOT EXISTS system_locked BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE recurring_expenses ADD COLUMN IF NOT EXISTS user_id VARCHAR(36)",
		"ALTER TABLE recurring_expenses ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'usd'",
		"ALTER TABLE recurring_expenses ADD COLUMN IF NOT EXISTS flow VARCHAR(20) NOT NULL DEFAULT 'expense'",
		"ALTER TABLE categories ADD COLUMN IF NOT EXISTS user_id VARCHAR(36)",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS target_balance NUMERIC(14, 2)",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS app_balance_before NUMERIC(14, 2)",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS note TEXT",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(120)",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'applied'",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"ALTER TABLE reconciliations ADD COLUMN IF NOT EXISTS reverted_at TIMESTAMPTZ",
		"ALTER TABLE password_resets ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE password_resets ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 5",
		"ALTER TABLE user_config ADD COLUMN IF NOT EXISTS plan_tier VARCHAR(20) NOT NULL DEFAULT 'free'",
		"ALTER TABLE telegram_user_links ADD COLUMN IF NOT EXISTS telegram_username TEXT",
		"ALTER TABLE telegram_user_links ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"ALTER TABLE telegram_user_links ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"ALTER TABLE telegram_link_codes ADD COLUMN IF NOT EXISTS used_at TIMESTAMPTZ",
		"ALTER TABLE telegram_link_codes ADD COLUMN IF NOT EXISTS used_by_telegram_user_id BIGINT",
		"ALTER TABLE telegram_link_codes ADD COLUMN IF NOT EXISTS used_telegram_username TEXT",
	}
	for _, stmt := range alterStmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`ALTER TABLE categories DROP CONSTRAINT IF EXISTS categories_name_key`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS categories_user_name_key ON categories (user_id, name)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS expenses_user_date_idx ON expenses (user_id, date DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS expenses_user_system_locked_idx ON expenses (user_id, system_locked)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS reconciliations_adjustment_expense_key ON reconciliations (adjustment_expense_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS reconciliations_reversal_expense_key ON reconciliations (reversal_expense_id) WHERE reversal_expense_id IS NOT NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS reconciliations_user_idempotency_key ON reconciliations (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS reconciliations_user_created_idx ON reconciliations (user_id, created_at DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS recurring_expenses_user_idx ON recurring_expenses (user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS categories_user_idx ON categories (user_id, position)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS password_resets_user_idx ON password_resets (user_id, created_at DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS email_verifications_user_idx ON email_verifications (user_id, created_at DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS email_verifications_token_hash_idx ON email_verifications (token_hash)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS oauth_identities_provider_key ON oauth_identities (provider, provider_user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS oauth_identities_user_idx ON oauth_identities (user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS telegram_user_links_user_key ON telegram_user_links (user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS telegram_user_links_telegram_key ON telegram_user_links (telegram_user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS telegram_link_codes_code_hash_key ON telegram_link_codes (code_hash)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS telegram_link_codes_user_created_idx ON telegram_link_codes (user_id, created_at DESC)`); err != nil {
		return err
	}
	if err := ensureRecurringInstanceUniqueIndex(db); err != nil {
		return err
	}
	if err := backfillLegacyReconciliations(db); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE expenses SET flow = CASE WHEN amount >= 0 THEN 'income' ELSE 'expense' END WHERE flow IS NULL OR flow = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE recurring_expenses SET flow = CASE WHEN amount >= 0 THEN 'income' ELSE 'expense' END WHERE flow IS NULL OR flow = ''`); err != nil {
		return err
	}
	if err := updateDefaultCategoriesToSpanish(db); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE user_config SET plan_tier = 'free' WHERE plan_tier IS NULL OR TRIM(plan_tier) = ''`); err != nil {
		return err
	}
	if err := ensureForeignKeys(db); err != nil {
		return err
	}
	if err := cleanupExpiredSessions(db); err != nil {
		return err
	}
	if err := cleanupExpiredPasswordResets(db); err != nil {
		return err
	}
	if err := cleanupExpiredEmailVerifications(db); err != nil {
		return err
	}
	return nil
}

func ensureRecurringInstanceUniqueIndex(db *sql.DB) error {
	var duplicateGroups int
	err := db.QueryRow(`
		SELECT COUNT(1) FROM (
			SELECT user_id, recurring_id, date
			FROM expenses
			WHERE recurring_id IS NOT NULL
			GROUP BY user_id, recurring_id, date
			HAVING COUNT(1) > 1
		) dup
	`).Scan(&duplicateGroups)
	if err != nil {
		return err
	}
	if duplicateGroups > 0 {
		log.Printf("[WARN] skipping unique recurring instance index: found %d duplicate groups in expenses", duplicateGroups)
		return nil
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS expenses_user_recurring_date_key ON expenses (user_id, recurring_id, date) WHERE recurring_id IS NOT NULL`); err != nil {
		log.Printf("[WARN] could not create recurring instance unique index: %v", err)
		return nil
	}
	return nil
}

func ensureForeignKeys(db *sql.DB) error {
	type fk struct {
		name      string
		table     string
		column    string
		refTable  string
		refColumn string
		onDelete  string
	}
	fks := []fk{
		{name: "sessions_user_fk", table: "sessions", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "password_resets_user_fk", table: "password_resets", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "email_verifications_user_fk", table: "email_verifications", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "oauth_identities_user_fk", table: "oauth_identities", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "user_config_user_fk", table: "user_config", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "categories_user_fk", table: "categories", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "expenses_user_fk", table: "expenses", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "recurring_expenses_user_fk", table: "recurring_expenses", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "reconciliations_user_fk", table: "reconciliations", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "telegram_user_links_user_fk", table: "telegram_user_links", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "telegram_link_codes_user_fk", table: "telegram_link_codes", column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
		{name: "reconciliations_adjustment_fk", table: "reconciliations", column: "adjustment_expense_id", refTable: "expenses", refColumn: "id", onDelete: "RESTRICT"},
		{name: "reconciliations_reversal_fk", table: "reconciliations", column: "reversal_expense_id", refTable: "expenses", refColumn: "id", onDelete: "RESTRICT"},
	}

	for _, fkDef := range fks {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`, fkDef.name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		var orphans int
		orphansQuery := fmt.Sprintf(
			`SELECT COUNT(1) FROM %s t LEFT JOIN %s r ON t.%s = r.%s WHERE t.%s IS NOT NULL AND r.%s IS NULL`,
			fkDef.table,
			fkDef.refTable,
			fkDef.column,
			fkDef.refColumn,
			fkDef.column,
			fkDef.refColumn,
		)
		if err := db.QueryRow(orphansQuery).Scan(&orphans); err != nil {
			return err
		}
		if orphans > 0 {
			log.Printf("[WARN] skipping FK %s: %d orphan rows", fkDef.name, orphans)
			continue
		}
		stmt := fmt.Sprintf(
			`ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s`,
			fkDef.table,
			fkDef.name,
			fkDef.column,
			fkDef.refTable,
			fkDef.refColumn,
			fkDef.onDelete,
		)
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func cleanupExpiredSessions(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

func cleanupExpiredPasswordResets(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM password_resets WHERE used_at IS NOT NULL OR expires_at < NOW()`)
	return err
}

func cleanupExpiredEmailVerifications(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM email_verifications WHERE verified_at IS NOT NULL OR expires_at < NOW()`)
	return err
}

const (
	systemOriginUser                     = "user"
	systemOriginReconciliationAdjustment = "reconciliation_adjustment"
	systemOriginReconciliationReversal   = "reconciliation_reversal"
	defaultResetMaxAttempts              = 5
)

func backfillLegacyReconciliations(db *sql.DB) error {
	type expenseRow struct {
		ID       string
		UserID   string
		Name     string
		Category string
		Amount   float64
		Currency string
		Date     time.Time
		TagsRaw  sql.NullString
	}

	normalize := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}
	isReconCategory := func(category string) bool {
		n := normalize(category)
		return n == "_conciliacion" || n == "conciliacion"
	}
	isAdjustmentName := func(name string) bool {
		return strings.Contains(normalize(name), "ajuste conciliacion")
	}
	isReversalName := func(name string) bool {
		return strings.Contains(normalize(name), "reversion ajuste conciliacion")
	}
	parseTags := func(raw sql.NullString) []string {
		if !raw.Valid || strings.TrimSpace(raw.String) == "" {
			return nil
		}
		var tags []string
		if err := json.Unmarshal([]byte(raw.String), &tags); err != nil {
			return nil
		}
		return tags
	}
	extractTagValue := func(tags []string, key string) string {
		key = strings.ToLower(strings.TrimSpace(key))
		for _, tag := range tags {
			raw := strings.TrimSpace(tag)
			lower := strings.ToLower(raw)
			if !strings.HasPrefix(lower, key) {
				continue
			}
			value := strings.TrimSpace(raw[len(key):])
			value = strings.TrimLeft(value, ":=_- ")
			if value != "" {
				return value
			}
		}
		return ""
	}
	parseTagFloat := func(tags []string, key string) *float64 {
		value := extractTagValue(tags, key)
		if value == "" {
			return nil
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil
		}
		return &v
	}

	rows, err := db.Query(`SELECT id, user_id, name, category, amount, currency, date, tags FROM expenses`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var adjustments []expenseRow
	var reversals []expenseRow
	for rows.Next() {
		var row expenseRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Category, &row.Amount, &row.Currency, &row.Date, &row.TagsRaw); err != nil {
			return err
		}
		if !isReconCategory(row.Category) {
			continue
		}
		if isReversalName(row.Name) {
			reversals = append(reversals, row)
			continue
		}
		if isAdjustmentName(row.Name) {
			adjustments = append(adjustments, row)
		}
	}

	for _, adj := range adjustments {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM reconciliations WHERE adjustment_expense_id = $1)`, adj.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			if _, err := db.Exec(`UPDATE expenses SET system_origin = $1, system_locked = TRUE WHERE id = $2`, systemOriginReconciliationAdjustment, adj.ID); err != nil {
				return err
			}
			continue
		}
		tags := parseTags(adj.TagsRaw)
		target := parseTagFloat(tags, "target")
		before := parseTagFloat(tags, "before")
		idem := extractTagValue(tags, "idem")
		note := extractTagValue(tags, "note")

		recID := uuid.New().String()
		if _, err := db.Exec(`
			INSERT INTO reconciliations (id, user_id, adjustment_expense_id, target_balance, app_balance_before, delta_amount, currency, note, idempotency_key, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), 'applied', $10)
			ON CONFLICT (adjustment_expense_id) DO NOTHING
		`, recID, adj.UserID, adj.ID, target, before, adj.Amount, strings.ToLower(strings.TrimSpace(adj.Currency)), note, idem, adj.Date); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE expenses SET system_origin = $1, system_locked = TRUE WHERE id = $2`, systemOriginReconciliationAdjustment, adj.ID); err != nil {
			return err
		}
	}

	for _, rev := range reversals {
		tags := parseTags(rev.TagsRaw)
		ref := strings.TrimSpace(extractTagValue(tags, "reversed"))
		if ref == "" {
			continue
		}
		if _, err := db.Exec(`
			UPDATE reconciliations
			SET reversal_expense_id = $1, status = 'reverted', reverted_at = COALESCE(reverted_at, $2)
			WHERE adjustment_expense_id = $3
		`, rev.ID, rev.Date, ref); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE expenses SET system_origin = $1, system_locked = TRUE WHERE id = $2`, systemOriginReconciliationReversal, rev.ID); err != nil {
			return err
		}
	}

	return nil
}

func updateDefaultCategoriesToSpanish(db *sql.DB) error {
	type catMap struct {
		from string
		to   string
	}
	mappings := []catMap{
		{from: "Food", to: "Comida"},
		{from: "Groceries", to: "Supermercado"},
		{from: "Travel", to: "Viajes"},
		{from: "Rent", to: "Alquiler"},
		{from: "Utilities", to: "Servicios"},
		{from: "Entertainment", to: "Entretenimiento"},
		{from: "Healthcare", to: "Salud"},
		{from: "Shopping", to: "Compras"},
		{from: "Miscellaneous", to: "Varios"},
		{from: "Income", to: "Ingresos"},
	}
	for _, mapping := range mappings {
		if _, err := db.Exec(
			`UPDATE categories c
			 SET name = $1
			 WHERE name = $2
			   AND NOT EXISTS (
			       SELECT 1 FROM categories c2
			       WHERE c2.user_id = c.user_id AND c2.name = $1
			   )`,
			mapping.to,
			mapping.from,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureBootstrapUser(db *sql.DB) error {
	email, password := bootstrapCredentials()
	if email == "" || password == "" {
		log.Printf("[BOOTSTRAP] skipped: missing BOOTSTRAP_EMAIL or BOOTSTRAP_PASSWORD")
		return nil
	}
	userID, err := ensureUser(db, email, password)
	if err != nil {
		return err
	}
	if err := backfillUserIDs(db, userID); err != nil {
		return err
	}
	legacyConfig, legacyErr := readLegacyConfig(db)
	if legacyErr != nil {
		legacyConfig.SetBaseConfig()
	}
	if err := ensureUserConfig(db, userID, &legacyConfig); err != nil {
		return err
	}
	if err := ensureUserCategories(db, userID, readLegacyCategories(db)); err != nil {
		return err
	}
	if err := setNotNullIfNoNulls(db, "expenses", "user_id"); err != nil {
		return err
	}
	if err := setNotNullIfNoNulls(db, "recurring_expenses", "user_id"); err != nil {
		return err
	}
	if err := setNotNullIfNoNulls(db, "categories", "user_id"); err != nil {
		return err
	}
	return nil
}

func bootstrapCredentials() (string, string) {
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_EMAIL"))
	password := strings.TrimSpace(os.Getenv("BOOTSTRAP_PASSWORD"))
	return strings.ToLower(email), password
}

func ensureUser(db *sql.DB, email, password string) (string, error) {
	var id string
	err := db.QueryRow(`SELECT id FROM users WHERE email = $1`, strings.ToLower(email)).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	hashed, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	id = uuid.New().String()
	if _, err := db.Exec(`INSERT INTO users (id, email, name, password_hash, status) VALUES ($1, $2, $3, $4, 'active')`, id, strings.ToLower(email), defaultUserName(strings.ToLower(email)), hashed); err != nil {
		return "", err
	}
	return id, nil
}

func ensureUserConfig(db *sql.DB, userID string, defaults *Config) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM user_config WHERE user_id = $1`, userID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	config := Config{}
	if defaults != nil && defaults.Currency != "" {
		config = *defaults
	} else {
		config.SetBaseConfig()
	}
	_, err := db.Exec(`INSERT INTO user_config (user_id, currency, start_date, plan_tier) VALUES ($1, $2, $3, $4)`, userID, config.Currency, config.StartDate, NormalizePlanTier(config.PlanTier))
	return err
}

func readLegacyConfig(db *sql.DB) (Config, error) {
	var config Config
	err := db.QueryRow(`SELECT currency, start_date FROM config WHERE id = 'default'`).Scan(&config.Currency, &config.StartDate)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func backfillUserIDs(db *sql.DB, userID string) error {
	stmts := []string{
		`UPDATE expenses SET user_id = $1 WHERE user_id IS NULL`,
		`UPDATE recurring_expenses SET user_id = $1 WHERE user_id IS NULL`,
		`UPDATE categories SET user_id = $1 WHERE user_id IS NULL`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt, userID); err != nil {
			return err
		}
	}
	return nil
}

func setNotNullIfNoNulls(db *sql.DB, table, column string) error {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(1) FROM %s WHERE %s IS NULL`, table, column)
	if err := db.QueryRow(query).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	alter := fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET NOT NULL`, table, column)
	if _, err := db.Exec(alter); err != nil {
		return err
	}
	return nil
}

func ensureUserCategories(db *sql.DB, userID string, seed []string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM categories WHERE user_id = $1`, userID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	categories := seed
	if len(categories) == 0 {
		categories = defaultCategories
	}
	return seedCategories(db, userID, categories)
}

func readLegacyCategories(db *sql.DB) []string {
	var categories []string
	var categoriesStr string
	if err := db.QueryRow(`SELECT categories FROM config WHERE id = 'default'`).Scan(&categoriesStr); err != nil {
		return nil
	}
	if err := json.Unmarshal([]byte(categoriesStr), &categories); err != nil {
		return nil
	}
	return categories
}

func seedCategories(db *sql.DB, userID string, categories []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for i, name := range categories {
		if _, err = tx.Exec(
			`INSERT INTO categories (user_id, name, position) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, name) DO UPDATE SET position = EXCLUDED.position`,
			userID, name, i+1,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *databaseStore) Close() error {
	return s.db.Close()
}

func (s *databaseStore) CreateUser(email, passwordHash string) (User, error) {
	return s.CreateUserWithStatus(email, passwordHash, "active")
}

func (s *databaseStore) CreateUserWithStatus(email, passwordHash, status string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user := User{
		ID:           uuid.New().String(),
		Name:         defaultUserName(email),
		Email:        email,
		PasswordHash: passwordHash,
		Status:       status,
	}
	query := `INSERT INTO users (id, email, name, password_hash, status) VALUES ($1, $2, $3, $4, $5) RETURNING created_at`
	var passwordArg any = user.PasswordHash
	if strings.TrimSpace(user.PasswordHash) == "" {
		passwordArg = nil
	}
	if err := s.db.QueryRow(query, user.ID, user.Email, user.Name, passwordArg, user.Status).Scan(&user.CreatedAt); err != nil {
		return User{}, err
	}
	if err := ensureUserConfig(s.db, user.ID, nil); err != nil {
		return User{}, err
	}
	if err := ensureUserCategories(s.db, user.ID, nil); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *databaseStore) GetUserByEmail(email string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	query := `SELECT id, email, COALESCE(name, ''), password_hash, status, created_at FROM users WHERE email = $1`
	var user User
	var passwordHash sql.NullString
	if err := s.db.QueryRow(query, email).Scan(&user.ID, &user.Email, &user.Name, &passwordHash, &user.Status, &user.CreatedAt); err != nil {
		return User{}, err
	}
	user.PasswordHash = strings.TrimSpace(passwordHash.String)
	return user, nil
}

func (s *databaseStore) GetUserByID(id string) (User, error) {
	query := `SELECT id, email, COALESCE(name, ''), password_hash, status, created_at FROM users WHERE id = $1`
	var user User
	var passwordHash sql.NullString
	if err := s.db.QueryRow(query, id).Scan(&user.ID, &user.Email, &user.Name, &passwordHash, &user.Status, &user.CreatedAt); err != nil {
		return User{}, err
	}
	user.PasswordHash = strings.TrimSpace(passwordHash.String)
	return user, nil
}

func (s *databaseStore) UpdateUserName(userID, name string) error {
	_, err := s.db.Exec(`UPDATE users SET name = $1 WHERE id = $2`, name, userID)
	return err
}

func (s *databaseStore) UpdateUserPassword(userID, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID)
	return err
}

func (s *databaseStore) UpdateUserStatus(userID, status string) error {
	_, err := s.db.Exec(`UPDATE users SET status = $1 WHERE id = $2`, status, userID)
	return err
}

func defaultUserName(email string) string {
	localPart := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	localPart = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(localPart)
	parts := strings.Fields(localPart)
	if len(parts) == 0 {
		return "Usuario"
	}
	for i, part := range parts {
		runes := []rune(strings.ToLower(part))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func (s *databaseStore) CreateOAuthIdentity(identity OAuthIdentity) error {
	identity.Provider = strings.ToLower(strings.TrimSpace(identity.Provider))
	identity.ProviderUserID = strings.TrimSpace(identity.ProviderUserID)
	identity.Email = strings.ToLower(strings.TrimSpace(identity.Email))
	if identity.ID == "" {
		identity.ID = uuid.New().String()
	}
	if identity.Provider == "" || identity.ProviderUserID == "" || identity.UserID == "" {
		return fmt.Errorf("invalid oauth identity")
	}
	_, err := s.db.Exec(
		`INSERT INTO oauth_identities (id, user_id, provider, provider_user_id, email) VALUES ($1, $2, $3, $4, $5)`,
		identity.ID,
		identity.UserID,
		identity.Provider,
		identity.ProviderUserID,
		identity.Email,
	)
	return err
}

func (s *databaseStore) GetOAuthIdentity(provider, providerUserID string) (OAuthIdentity, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerUserID = strings.TrimSpace(providerUserID)
	var identity OAuthIdentity
	err := s.db.QueryRow(
		`SELECT id, user_id, provider, provider_user_id, COALESCE(email, ''), created_at
		 FROM oauth_identities
		 WHERE provider = $1 AND provider_user_id = $2`,
		provider,
		providerUserID,
	).Scan(
		&identity.ID,
		&identity.UserID,
		&identity.Provider,
		&identity.ProviderUserID,
		&identity.Email,
		&identity.CreatedAt,
	)
	if err != nil {
		return OAuthIdentity{}, err
	}
	return identity, nil
}

func (s *databaseStore) CreateSession(session Session) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	query := `INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := s.db.Exec(query, session.ID, session.UserID, session.CreatedAt, session.ExpiresAt, session.IP, session.UserAgent)
	return err
}

func (s *databaseStore) GetSession(id string) (Session, error) {
	query := `SELECT id, user_id, created_at, expires_at, ip, user_agent FROM sessions WHERE id = $1`
	var session Session
	if err := s.db.QueryRow(query, id).Scan(&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt, &session.IP, &session.UserAgent); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *databaseStore) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func (s *databaseStore) DeleteSessionsByUserID(userID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *databaseStore) CreatePasswordReset(reset PasswordReset) error {
	if reset.ID == "" {
		reset.ID = uuid.New().String()
	}
	if reset.CreatedAt.IsZero() {
		reset.CreatedAt = time.Now()
	}
	if reset.MaxAttempts <= 0 {
		reset.MaxAttempts = defaultResetMaxAttempts
	}
	_, err := s.db.Exec(`DELETE FROM password_resets WHERE user_id = $1 AND used_at IS NULL`, reset.UserID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO password_resets (id, user_id, code_hash, attempts, max_attempts, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		reset.ID,
		reset.UserID,
		reset.CodeHash,
		0,
		reset.MaxAttempts,
		reset.CreatedAt,
		reset.ExpiresAt,
	)
	return err
}

func (s *databaseStore) GetLatestPasswordReset(userID string) (PasswordReset, error) {
	query := `SELECT id, user_id, code_hash, attempts, max_attempts, created_at, expires_at FROM password_resets WHERE user_id = $1 AND used_at IS NULL ORDER BY created_at DESC LIMIT 1`
	var reset PasswordReset
	if err := s.db.QueryRow(query, userID).Scan(&reset.ID, &reset.UserID, &reset.CodeHash, &reset.Attempts, &reset.MaxAttempts, &reset.CreatedAt, &reset.ExpiresAt); err != nil {
		return PasswordReset{}, err
	}
	return reset, nil
}

func (s *databaseStore) MarkPasswordResetUsed(resetID string) error {
	_, err := s.db.Exec(`UPDATE password_resets SET used_at = NOW() WHERE id = $1`, resetID)
	return err
}

func (s *databaseStore) RegisterPasswordResetFailure(resetID string) (attempts int, maxAttempts int, exhausted bool, err error) {
	row := s.db.QueryRow(`
		WITH updated AS (
			UPDATE password_resets
			SET attempts = attempts + 1,
			    used_at = CASE
			        WHEN (attempts + 1) >= max_attempts THEN NOW()
			        ELSE used_at
			    END
			WHERE id = $1
			  AND used_at IS NULL
			RETURNING attempts, max_attempts, used_at
		)
		SELECT attempts, max_attempts, used_at IS NOT NULL
		FROM updated
	`, resetID)
	err = row.Scan(&attempts, &maxAttempts, &exhausted)
	return attempts, maxAttempts, exhausted, err
}

func (s *databaseStore) CreateEmailVerification(verification EmailVerification) error {
	if verification.ID == "" {
		verification.ID = uuid.New().String()
	}
	if verification.CreatedAt.IsZero() {
		verification.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(`DELETE FROM email_verifications WHERE user_id = $1 AND verified_at IS NULL`, verification.UserID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO email_verifications (id, user_id, token_hash, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		verification.ID,
		verification.UserID,
		verification.TokenHash,
		verification.CreatedAt,
		verification.ExpiresAt,
	)
	return err
}

func (s *databaseStore) GetEmailVerificationByTokenHash(tokenHash string) (EmailVerification, error) {
	query := `SELECT id, user_id, token_hash, created_at, expires_at, verified_at FROM email_verifications WHERE token_hash = $1 ORDER BY created_at DESC LIMIT 1`
	var verification EmailVerification
	var verifiedAt sql.NullTime
	if err := s.db.QueryRow(query, tokenHash).Scan(&verification.ID, &verification.UserID, &verification.TokenHash, &verification.CreatedAt, &verification.ExpiresAt, &verifiedAt); err != nil {
		return EmailVerification{}, err
	}
	if verifiedAt.Valid {
		verification.VerifiedAt = verifiedAt.Time
	}
	return verification, nil
}

func (s *databaseStore) MarkEmailVerificationUsed(verificationID string) error {
	_, err := s.db.Exec(`UPDATE email_verifications SET verified_at = NOW() WHERE id = $1`, verificationID)
	return err
}

func (s *databaseStore) GetConfig(userID string) (*Config, error) {
	currency, startDate, planTier, err := s.getOrCreateUserConfig(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user config: %v", err)
	}
	categories, err := s.GetCategories(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get categories from db: %v", err)
	}
	recurring, err := s.GetRecurringExpenses(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get recurring expenses for config: %v", err)
	}

	return &Config{
		Categories:        categories,
		Currency:          currency,
		StartDate:         startDate,
		PlanTier:          planTier,
		RecurringExpenses: recurring,
	}, nil
}

func (s *databaseStore) GetUserPlanTier(userID string) (string, error) {
	var planTier string
	err := s.db.QueryRow(`SELECT COALESCE(plan_tier, 'free') FROM user_config WHERE user_id = $1`, userID).Scan(&planTier)
	if err == sql.ErrNoRows {
		return PlanTierFree, nil
	}
	if err != nil {
		return "", err
	}
	return NormalizePlanTier(planTier), nil
}

func (s *databaseStore) getOrCreateUserConfig(userID string) (string, int, string, error) {
	var currency string
	var startDate int
	var planTier string
	err := s.db.QueryRow(`SELECT currency, start_date, COALESCE(plan_tier, 'free') FROM user_config WHERE user_id = $1`, userID).Scan(&currency, &startDate, &planTier)
	if err == nil {
		return currency, startDate, NormalizePlanTier(planTier), nil
	}
	if err != sql.ErrNoRows {
		return "", 0, "", err
	}
	config := Config{}
	config.SetBaseConfig()
	if _, err := s.db.Exec(`INSERT INTO user_config (user_id, currency, start_date, plan_tier) VALUES ($1, $2, $3, $4)`, userID, config.Currency, config.StartDate, NormalizePlanTier(config.PlanTier)); err != nil {
		return "", 0, "", err
	}
	return config.Currency, config.StartDate, NormalizePlanTier(config.PlanTier), nil
}

func scanTelegramUserLink(scanner interface{ Scan(...any) error }) (TelegramUserLink, error) {
	var link TelegramUserLink
	var username sql.NullString
	if err := scanner.Scan(&link.ID, &link.UserID, &link.TelegramUserID, &username, &link.CreatedAt, &link.UpdatedAt); err != nil {
		return TelegramUserLink{}, err
	}
	if username.Valid {
		link.TelegramUsername = username.String
	}
	return link, nil
}

func scanTelegramLinkCode(scanner interface{ Scan(...any) error }) (TelegramLinkCode, error) {
	var code TelegramLinkCode
	var usedAt sql.NullTime
	var usedBy sql.NullInt64
	var usedUsername sql.NullString
	if err := scanner.Scan(
		&code.ID,
		&code.UserID,
		&code.CodeHash,
		&code.CreatedAt,
		&code.ExpiresAt,
		&usedAt,
		&usedBy,
		&usedUsername,
	); err != nil {
		return TelegramLinkCode{}, err
	}
	if usedAt.Valid {
		code.UsedAt = &usedAt.Time
	}
	if usedBy.Valid {
		v := usedBy.Int64
		code.UsedByTelegramUserID = &v
	}
	if usedUsername.Valid {
		code.UsedTelegramUsername = usedUsername.String
	}
	return code, nil
}

func (s *databaseStore) GetTelegramUserLinkByUserID(userID string) (TelegramUserLink, error) {
	query := `
		SELECT id, user_id, telegram_user_id, telegram_username, created_at, updated_at
		FROM telegram_user_links
		WHERE user_id = $1
	`
	link, err := scanTelegramUserLink(s.db.QueryRow(query, userID))
	if err != nil {
		return TelegramUserLink{}, err
	}
	return link, nil
}

func (s *databaseStore) GetTelegramUserLinkByTelegramUserID(telegramUserID int64) (TelegramUserLink, error) {
	query := `
		SELECT id, user_id, telegram_user_id, telegram_username, created_at, updated_at
		FROM telegram_user_links
		WHERE telegram_user_id = $1
	`
	link, err := scanTelegramUserLink(s.db.QueryRow(query, telegramUserID))
	if err != nil {
		return TelegramUserLink{}, err
	}
	return link, nil
}

func (s *databaseStore) GetActiveTelegramLinkCode(userID string, now time.Time) (TelegramLinkCode, error) {
	query := `
		SELECT id, user_id, code_hash, created_at, expires_at, used_at, used_by_telegram_user_id, used_telegram_username
		FROM telegram_link_codes
		WHERE user_id = $1
		  AND used_at IS NULL
		  AND expires_at > $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	code, err := scanTelegramLinkCode(s.db.QueryRow(query, userID, now))
	if err != nil {
		return TelegramLinkCode{}, err
	}
	return code, nil
}

func (s *databaseStore) InvalidateActiveTelegramLinkCodes(userID string, usedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE telegram_link_codes
		 SET used_at = $1
		 WHERE user_id = $2
		   AND used_at IS NULL
		   AND expires_at > $1`,
		usedAt, userID,
	)
	return err
}

func (s *databaseStore) CreateTelegramLinkCode(userID, codeHash string, expiresAt, createdAt time.Time) (TelegramLinkCode, error) {
	code := TelegramLinkCode{
		ID:        uuid.New().String(),
		UserID:    userID,
		CodeHash:  codeHash,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}
	query := `
		INSERT INTO telegram_link_codes (
			id, user_id, code_hash, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := s.db.Exec(query, code.ID, code.UserID, code.CodeHash, code.CreatedAt, code.ExpiresAt); err != nil {
		return TelegramLinkCode{}, err
	}
	return code, nil
}

func isUniqueConstraintViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	if string(pqErr.Code) != "23505" {
		return false
	}
	if constraint == "" {
		return true
	}
	return pqErr.Constraint == constraint
}

func (s *databaseStore) ConsumeTelegramLinkCode(codeHash string, telegramUserID int64, telegramUsername string, now time.Time) (TelegramUserLink, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramUserLink{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	lockCodeQuery := `
		SELECT id, user_id, code_hash, created_at, expires_at, used_at, used_by_telegram_user_id, used_telegram_username
		FROM telegram_link_codes
		WHERE code_hash = $1
		LIMIT 1
		FOR UPDATE
	`
	code, err := scanTelegramLinkCode(tx.QueryRow(lockCodeQuery, codeHash))
	if err != nil {
		if err == sql.ErrNoRows {
			return TelegramUserLink{}, ErrTelegramInvalidLinkCode
		}
		return TelegramUserLink{}, err
	}
	if code.UsedAt != nil {
		return TelegramUserLink{}, ErrTelegramLinkCodeUsed
	}
	if !code.ExpiresAt.After(now) {
		return TelegramUserLink{}, ErrTelegramLinkCodeExpired
	}

	targetUserID := code.UserID
	var planTier string
	planErr := tx.QueryRow(
		`SELECT COALESCE(plan_tier, 'free') FROM user_config WHERE user_id = $1`,
		targetUserID,
	).Scan(&planTier)
	if planErr != nil && planErr != sql.ErrNoRows {
		return TelegramUserLink{}, planErr
	}
	if NormalizePlanTier(planTier) != PlanTierPremium {
		return TelegramUserLink{}, ErrTelegramPremiumRequired
	}

	var existingUserID string
	tgRowErr := tx.QueryRow(
		`SELECT user_id FROM telegram_user_links WHERE telegram_user_id = $1 FOR UPDATE`,
		telegramUserID,
	).Scan(&existingUserID)
	if tgRowErr != nil && tgRowErr != sql.ErrNoRows {
		return TelegramUserLink{}, tgRowErr
	}
	if tgRowErr == nil && existingUserID != targetUserID {
		return TelegramUserLink{}, ErrTelegramUserAlreadyLinked
	}

	var existingTelegramID int64
	userRowErr := tx.QueryRow(
		`SELECT telegram_user_id FROM telegram_user_links WHERE user_id = $1 FOR UPDATE`,
		targetUserID,
	).Scan(&existingTelegramID)
	if userRowErr != nil && userRowErr != sql.ErrNoRows {
		return TelegramUserLink{}, userRowErr
	}
	if userRowErr == nil && existingTelegramID != telegramUserID {
		return TelegramUserLink{}, ErrTelegramAlreadyLinked
	}

	upsertQuery := `
		INSERT INTO telegram_user_links (
			id, user_id, telegram_user_id, telegram_username, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (user_id)
		DO UPDATE SET
			telegram_user_id = EXCLUDED.telegram_user_id,
			telegram_username = EXCLUDED.telegram_username,
			updated_at = EXCLUDED.updated_at
	`
	if _, err := tx.Exec(upsertQuery, uuid.New().String(), targetUserID, telegramUserID, strings.TrimSpace(telegramUsername), now); err != nil {
		if isUniqueConstraintViolation(err, "telegram_user_links_telegram_key") {
			return TelegramUserLink{}, ErrTelegramUserAlreadyLinked
		}
		if isUniqueConstraintViolation(err, "telegram_user_links_user_key") {
			return TelegramUserLink{}, ErrTelegramAlreadyLinked
		}
		return TelegramUserLink{}, err
	}

	markUsedQuery := `
		UPDATE telegram_link_codes
		SET used_at = $1, used_by_telegram_user_id = $2, used_telegram_username = $3
		WHERE id = $4
		  AND used_at IS NULL
	`
	result, err := tx.Exec(markUsedQuery, now, telegramUserID, strings.TrimSpace(telegramUsername), code.ID)
	if err != nil {
		return TelegramUserLink{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return TelegramUserLink{}, err
	}
	if rowsAffected == 0 {
		return TelegramUserLink{}, ErrTelegramLinkCodeUsed
	}

	linkQuery := `
		SELECT id, user_id, telegram_user_id, telegram_username, created_at, updated_at
		FROM telegram_user_links
		WHERE user_id = $1
	`
	link, err := scanTelegramUserLink(tx.QueryRow(linkQuery, targetUserID))
	if err != nil {
		return TelegramUserLink{}, err
	}

	if err := tx.Commit(); err != nil {
		return TelegramUserLink{}, err
	}
	return link, nil
}

func (s *databaseStore) GetCategories(userID string) ([]string, error) {
	categories, err := s.getCategoriesFromTable(userID)
	if err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		categories = defaultCategories
		if seedErr := seedCategories(s.db, userID, categories); seedErr != nil {
			return nil, seedErr
		}
	}
	return categories, nil
}

func (s *databaseStore) UpdateCategories(userID string, categories []string) error {
	if err := s.updateCategoriesTable(userID, categories); err != nil {
		return err
	}
	return nil
}

func (s *databaseStore) getCategoriesFromTable(userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM categories WHERE user_id = $1 ORDER BY position ASC`, userID)
	if err != nil {
		log.Printf("[DEBUG] getCategoriesFromTable query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("[DEBUG] getCategoriesFromTable scan error: %v", err)
			return nil, err
		}
		categories = append(categories, name)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[DEBUG] getCategoriesFromTable rows error: %v", err)
		return nil, err
	}
	log.Printf("[DEBUG] getCategoriesFromTable returned %d categories: %v", len(categories), categories)
	return categories, nil
}

func (s *databaseStore) updateCategoriesTable(userID string, categories []string) error {
	if len(categories) == 0 {
		return fmt.Errorf("categories cannot be empty")
	}

	// Validate that no category is empty
	for _, cat := range categories {
		if strings.TrimSpace(cat) == "" {
			return fmt.Errorf("category names cannot be empty")
		}
	}

	log.Printf("[DEBUG] updateCategoriesTable called with %d categories: %v", len(categories), categories)

	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("[DEBUG] updateCategoriesTable begin transaction error: %v", err)
		return err
	}
	defer func() {
		if err != nil {
			log.Printf("[DEBUG] updateCategoriesTable rolling back transaction due to error: %v", err)
			_ = tx.Rollback()
		}
	}()

	for i, name := range categories {
		log.Printf("[DEBUG] updateCategoriesTable inserting category %d: %s", i+1, name)
		if _, err = tx.Exec(
			`INSERT INTO categories (user_id, name, position) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, name) DO UPDATE SET position = EXCLUDED.position`,
			userID, name, i+1,
		); err != nil {
			log.Printf("[DEBUG] updateCategoriesTable insert error for category %s: %v", name, err)
			return err
		}
	}

	log.Printf("[DEBUG] updateCategoriesTable deleting categories not in list")
	// Delete categories that are not in the new list
	// Using a safer approach with explicit list building
	if _, err = tx.Exec(`DELETE FROM categories WHERE user_id = $1 AND NOT (name = ANY($2))`, userID, pq.Array(categories)); err != nil {
		log.Printf("[DEBUG] updateCategoriesTable delete error: %v", err)
		return fmt.Errorf("failed to delete removed categories: %v", err)
	}

	if err = tx.Commit(); err != nil {
		log.Printf("[DEBUG] updateCategoriesTable commit error: %v", err)
		return fmt.Errorf("failed to commit category update: %v", err)
	}

	log.Printf("[DEBUG] updateCategoriesTable successfully updated categories")
	return nil
}

func (s *databaseStore) GetCurrency(userID string) (string, error) {
	currency, _, _, err := s.getOrCreateUserConfig(userID)
	if err != nil {
		return "", err
	}
	return currency, nil
}

func (s *databaseStore) UpdateCurrency(userID string, currency string) error {
	if !slices.Contains(SupportedCurrencies, currency) {
		return fmt.Errorf("invalid currency: %s", currency)
	}
	_, startDate, planTier, err := s.getOrCreateUserConfig(userID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO user_config (user_id, currency, start_date, plan_tier)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET currency = EXCLUDED.currency`,
		userID, currency, startDate, planTier,
	)
	return err
}

func (s *databaseStore) GetStartDate(userID string) (int, error) {
	_, startDate, _, err := s.getOrCreateUserConfig(userID)
	if err != nil {
		return 0, err
	}
	return startDate, nil
}

func (s *databaseStore) UpdateStartDate(userID string, startDate int) error {
	if startDate < 1 || startDate > 31 {
		return fmt.Errorf("invalid start date: %d", startDate)
	}
	currency, _, planTier, err := s.getOrCreateUserConfig(userID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO user_config (user_id, currency, start_date, plan_tier)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET start_date = EXCLUDED.start_date`,
		userID, currency, startDate, planTier,
	)
	return err
}

func scanExpense(scanner interface{ Scan(...any) error }) (Expense, error) {
	var expense Expense
	var tagsStr sql.NullString
	var recurringID sql.NullString
	var source sql.NullString
	var card sql.NullString
	var systemOrigin sql.NullString
	var systemLocked sql.NullBool
	err := scanner.Scan(
		&expense.ID,
		&recurringID,
		&expense.Name,
		&expense.Category,
		&expense.Amount,
		&expense.Currency,
		&expense.Date,
		&expense.Flow,
		&tagsStr,
		&source,
		&card,
		&systemOrigin,
		&systemLocked,
	)
	if err != nil {
		return Expense{}, err
	}
	if recurringID.Valid {
		expense.RecurringID = recurringID.String
	}
	if source.Valid {
		expense.Source = source.String
	}
	if card.Valid {
		expense.Card = card.String
	}
	if systemOrigin.Valid {
		expense.SystemOrigin = systemOrigin.String
	}
	if systemLocked.Valid {
		expense.SystemLocked = systemLocked.Bool
	}
	if tagsStr.Valid && tagsStr.String != "" {
		if err := json.Unmarshal([]byte(tagsStr.String), &expense.Tags); err != nil {
			return Expense{}, fmt.Errorf("failed to parse tags for expense %s: %v", expense.ID, err)
		}
	}
	return expense, nil
}

func (s *databaseStore) GetAllExpenses(userID string) ([]Expense, error) {
	query := `SELECT id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked FROM expenses WHERE user_id = $1 ORDER BY date DESC`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query expenses: %v", err)
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		expense, err := scanExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan expense: %v", err)
		}
		expenses = append(expenses, expense)
	}
	return expenses, nil
}

func (s *databaseStore) GetExpense(userID, id string) (Expense, error) {
	query := `SELECT id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked FROM expenses WHERE user_id = $1 AND id = $2`
	expense, err := scanExpense(s.db.QueryRow(query, userID, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return Expense{}, fmt.Errorf("expense with ID %s not found", id)
		}
		return Expense{}, fmt.Errorf("failed to get expense: %v", err)
	}
	return expense, nil
}

func (s *databaseStore) AddExpense(userID string, expense Expense) error {
	if expense.ID == "" {
		expense.ID = uuid.New().String()
	}
	if expense.Flow == "" {
		if expense.Amount >= 0 {
			expense.Flow = "income"
		} else {
			expense.Flow = "expense"
		}
	}
	if expense.Currency == "" {
		if currency, err := s.GetCurrency(userID); err == nil {
			expense.Currency = currency
		}
	}
	if expense.Date.IsZero() {
		expense.Date = time.Now()
	}
	expense.SystemOrigin = strings.ToLower(strings.TrimSpace(expense.SystemOrigin))
	if expense.SystemOrigin == "" {
		expense.SystemOrigin = systemOriginUser
	}
	expense.SystemLocked = false
	tagsJSON, err := json.Marshal(expense.Tags)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO expenses (id, user_id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	_, err = s.db.Exec(query, expense.ID, userID, expense.RecurringID, expense.Name, expense.Category, expense.Amount, expense.Currency, expense.Date, expense.Flow, string(tagsJSON), expense.Source, expense.Card, expense.SystemOrigin, expense.SystemLocked)
	return err
}

func (s *databaseStore) UpdateExpense(userID, id string, expense Expense) error {
	var isLocked bool
	if err := s.db.QueryRow(`SELECT system_locked FROM expenses WHERE user_id = $1 AND id = $2`, userID, id).Scan(&isLocked); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("expense with ID %s not found", id)
		}
		return fmt.Errorf("failed to check expense lock: %v", err)
	}
	if isLocked {
		return ErrSystemLockedExpense
	}

	tagsJSON, err := json.Marshal(expense.Tags)
	if err != nil {
		return err
	}
	// TODO: revisit to maybe remove this later, might not be a good default for update
	if expense.Currency == "" {
		if currency, err := s.GetCurrency(userID); err == nil {
			expense.Currency = currency
		}
	}
	if expense.Flow == "" {
		if expense.Amount >= 0 {
			expense.Flow = "income"
		} else {
			expense.Flow = "expense"
		}
	}
	query := `
		UPDATE expenses
		SET name = $1, category = $2, amount = $3, currency = $4, date = $5, flow = $6, tags = $7, recurring_id = $8, source = $9, card = $10
		WHERE user_id = $11 AND id = $12
	`
	result, err := s.db.Exec(query, expense.Name, expense.Category, expense.Amount, expense.Currency, expense.Date, expense.Flow, string(tagsJSON), expense.RecurringID, expense.Source, expense.Card, userID, id)
	if err != nil {
		return fmt.Errorf("failed to update expense: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("expense with ID %s not found", id)
	}
	return nil
}

func (s *databaseStore) RemoveExpense(userID, id string) error {
	var isLocked bool
	if err := s.db.QueryRow(`SELECT system_locked FROM expenses WHERE user_id = $1 AND id = $2`, userID, id).Scan(&isLocked); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("expense with ID %s not found", id)
		}
		return fmt.Errorf("failed to check expense lock: %v", err)
	}
	if isLocked {
		return ErrSystemLockedExpense
	}

	query := `DELETE FROM expenses WHERE user_id = $1 AND id = $2`
	result, err := s.db.Exec(query, userID, id)
	if err != nil {
		return fmt.Errorf("failed to delete expense: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("expense with ID %s not found", id)
	}
	return nil
}

func (s *databaseStore) AddMultipleExpenses(userID string, expenses []Expense) error {
	if len(expenses) == 0 {
		return nil
	}
	// use the same addexpense method
	for _, exp := range expenses {
		if err := s.AddExpense(userID, exp); err != nil {
			return err
		}
	}
	return nil
}

func (s *databaseStore) RemoveMultipleExpenses(userID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var lockedCount int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM expenses WHERE user_id = $1 AND id = ANY($2) AND system_locked = TRUE`, userID, pq.Array(ids)).Scan(&lockedCount); err != nil {
		return fmt.Errorf("failed to validate system-locked expenses: %v", err)
	}
	if lockedCount > 0 {
		return ErrSystemLockedExpense
	}
	query := `DELETE FROM expenses WHERE user_id = $1 AND id = ANY($2)`
	_, err := s.db.Exec(query, userID, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("failed to delete multiple expenses: %v", err)
	}
	return nil
}

func normalizeCurrencyCode(currency string) string {
	normalized := strings.ToLower(strings.TrimSpace(currency))
	if normalized == "" {
		return "ars"
	}
	return normalized
}

func (s *databaseStore) userCurrencyTx(tx *sql.Tx, userID string) (string, error) {
	var currency string
	if err := tx.QueryRow(`SELECT currency FROM user_config WHERE user_id = $1`, userID).Scan(&currency); err != nil {
		if err == sql.ErrNoRows {
			return "ars", nil
		}
		return "", err
	}
	return normalizeCurrencyCode(currency), nil
}

func (s *databaseStore) currentCABalanceTx(tx *sql.Tx, userID, currency string, now time.Time) (float64, error) {
	var balance float64
	if err := tx.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE user_id = $1
		  AND LOWER(COALESCE(currency, '')) = LOWER($2)
		  AND (source IS NULL OR source = '' OR UPPER(source) = 'CA')
		  AND date <= $3
	`, userID, currency, now).Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

func scanReconciliationRecord(scanner interface{ Scan(...any) error }) (ReconciliationRecord, error) {
	var rec ReconciliationRecord
	var target sql.NullFloat64
	var before sql.NullFloat64
	var note sql.NullString
	var idem sql.NullString
	var reversalID sql.NullString
	var revertedAt sql.NullTime
	if err := scanner.Scan(
		&rec.ID,
		&rec.UserID,
		&rec.AdjustmentExpenseID,
		&reversalID,
		&target,
		&before,
		&rec.DeltaAmount,
		&rec.Currency,
		&note,
		&idem,
		&rec.Status,
		&rec.CreatedAt,
		&revertedAt,
	); err != nil {
		return ReconciliationRecord{}, err
	}
	if target.Valid {
		v := target.Float64
		rec.TargetBalance = &v
	}
	if before.Valid {
		v := before.Float64
		rec.AppBalanceBefore = &v
	}
	if note.Valid {
		rec.Note = note.String
	}
	if idem.Valid {
		rec.IdempotencyKey = idem.String
	}
	if reversalID.Valid {
		rec.ReversalExpenseID = reversalID.String
	}
	if revertedAt.Valid {
		v := revertedAt.Time
		rec.RevertedAt = &v
	}
	return rec, nil
}

func (s *databaseStore) getReconciliationByIdempotencyTx(tx *sql.Tx, userID, idempotencyKey string) (ReconciliationRecord, error) {
	query := `
		SELECT id, user_id, adjustment_expense_id, reversal_expense_id, target_balance, app_balance_before, delta_amount, currency, note, idempotency_key, status, created_at, reverted_at
		FROM reconciliations
		WHERE user_id = $1 AND idempotency_key = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	return scanReconciliationRecord(tx.QueryRow(query, userID, idempotencyKey))
}

func (s *databaseStore) getExpenseTx(tx *sql.Tx, userID, id string) (Expense, error) {
	query := `SELECT id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked FROM expenses WHERE user_id = $1 AND id = $2`
	expense, err := scanExpense(tx.QueryRow(query, userID, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return Expense{}, fmt.Errorf("expense with ID %s not found", id)
		}
		return Expense{}, err
	}
	return expense, nil
}

func (s *databaseStore) insertSystemExpenseTx(tx *sql.Tx, userID string, expense Expense) error {
	if expense.ID == "" {
		expense.ID = uuid.New().String()
	}
	if expense.Flow == "" {
		if expense.Amount >= 0 {
			expense.Flow = "income"
		} else {
			expense.Flow = "expense"
		}
	}
	if expense.Currency == "" {
		expense.Currency = "ars"
	}
	if expense.Date.IsZero() {
		expense.Date = time.Now()
	}
	tagsJSON, err := json.Marshal(expense.Tags)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO expenses (id, user_id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, expense.ID, userID, expense.RecurringID, expense.Name, expense.Category, expense.Amount, expense.Currency, expense.Date, expense.Flow, string(tagsJSON), expense.Source, expense.Card, expense.SystemOrigin, expense.SystemLocked)
	return err
}

func (s *databaseStore) ApplyReconciliation(userID string, input ReconciliationApplyInput) (ReconciliationApplyResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ReconciliationApplyResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	currency := normalizeCurrencyCode(input.Currency)
	if strings.TrimSpace(input.Currency) == "" {
		currency, err = s.userCurrencyTx(tx, userID)
		if err != nil {
			return ReconciliationApplyResult{}, err
		}
	}

	currentBalance, err := s.currentCABalanceTx(tx, userID, currency, now)
	if err != nil {
		return ReconciliationApplyResult{}, err
	}

	difference := input.TargetBalance - currentBalance
	if math.Abs(difference) < 0.005 {
		if err := tx.Commit(); err != nil {
			return ReconciliationApplyResult{}, err
		}
		return ReconciliationApplyResult{
			Status:         "noop",
			CurrentBalance: currentBalance,
			Difference:     0,
			Currency:       currency,
		}, nil
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey != "" {
		existing, findErr := s.getReconciliationByIdempotencyTx(tx, userID, idempotencyKey)
		if findErr == nil {
			exp, getErr := s.getExpenseTx(tx, userID, existing.AdjustmentExpenseID)
			if getErr != nil {
				return ReconciliationApplyResult{}, getErr
			}
			if err := tx.Commit(); err != nil {
				return ReconciliationApplyResult{}, err
			}
			return ReconciliationApplyResult{
				Status:         "duplicate",
				Expense:        exp,
				CurrentBalance: currentBalance,
				Difference:     difference,
				Currency:       currency,
			}, nil
		}
		if !errors.Is(findErr, sql.ErrNoRows) {
			return ReconciliationApplyResult{}, findErr
		}
	}

	flow := "expense"
	if difference > 0 {
		flow = "income"
	}
	adjustment := Expense{
		ID:           uuid.New().String(),
		Name:         "Ajuste conciliacion CA",
		Category:     "_Conciliacion",
		Amount:       difference,
		Currency:     currency,
		Source:       "CA",
		Card:         "",
		Flow:         flow,
		Date:         now,
		SystemOrigin: systemOriginReconciliationAdjustment,
		SystemLocked: true,
	}
	if err := adjustment.Validate(); err != nil {
		return ReconciliationApplyResult{}, err
	}
	if err := s.insertSystemExpenseTx(tx, userID, adjustment); err != nil {
		return ReconciliationApplyResult{}, err
	}

	recID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO reconciliations (id, user_id, adjustment_expense_id, target_balance, app_balance_before, delta_amount, currency, note, idempotency_key, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), 'applied', $10)
	`, recID, userID, adjustment.ID, input.TargetBalance, currentBalance, difference, currency, strings.TrimSpace(input.Note), idempotencyKey, now)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == "23505" && strings.Contains(strings.ToLower(pgErr.Constraint), "reconciliations_user_idempotency_key") {
			existing, findErr := s.getReconciliationByIdempotencyTx(tx, userID, idempotencyKey)
			if findErr != nil {
				return ReconciliationApplyResult{}, err
			}
			exp, getErr := s.getExpenseTx(tx, userID, existing.AdjustmentExpenseID)
			if getErr != nil {
				return ReconciliationApplyResult{}, getErr
			}
			if err := tx.Commit(); err != nil {
				return ReconciliationApplyResult{}, err
			}
			return ReconciliationApplyResult{
				Status:         "duplicate",
				Expense:        exp,
				CurrentBalance: currentBalance,
				Difference:     difference,
				Currency:       currency,
			}, nil
		}
		return ReconciliationApplyResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return ReconciliationApplyResult{}, err
	}

	return ReconciliationApplyResult{
		Status:         "applied",
		Expense:        adjustment,
		CurrentBalance: currentBalance,
		Difference:     difference,
		Currency:       currency,
	}, nil
}

func (s *databaseStore) GetReconciliationHistory(userID string) ([]ReconciliationRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, adjustment_expense_id, reversal_expense_id, target_balance, app_balance_before, delta_amount, currency, note, idempotency_key, status, created_at, reverted_at
		FROM reconciliations
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ReconciliationRecord, 0)
	for rows.Next() {
		rec, scanErr := scanReconciliationRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, rec)
	}
	return items, nil
}

func (s *databaseStore) RevertReconciliation(userID, adjustmentExpenseID string, now time.Time) (Expense, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Expense{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if now.IsZero() {
		now = time.Now()
	}

	rec, err := scanReconciliationRecord(tx.QueryRow(`
		SELECT id, user_id, adjustment_expense_id, reversal_expense_id, target_balance, app_balance_before, delta_amount, currency, note, idempotency_key, status, created_at, reverted_at
		FROM reconciliations
		WHERE user_id = $1 AND adjustment_expense_id = $2
		LIMIT 1
	`, userID, adjustmentExpenseID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Expense{}, ErrReconciliationNotFound
		}
		return Expense{}, err
	}
	if strings.EqualFold(rec.Status, "reverted") || strings.TrimSpace(rec.ReversalExpenseID) != "" {
		return Expense{}, ErrReconciliationAlreadyReverted
	}

	adjustment, err := s.getExpenseTx(tx, userID, rec.AdjustmentExpenseID)
	if err != nil {
		return Expense{}, err
	}

	reversalAmount := -adjustment.Amount
	flow := "expense"
	if reversalAmount > 0 {
		flow = "income"
	}
	reversal := Expense{
		ID:           uuid.New().String(),
		Name:         "Reversion ajuste conciliacion CA",
		Category:     "_Conciliacion",
		Amount:       reversalAmount,
		Currency:     normalizeCurrencyCode(adjustment.Currency),
		Source:       "CA",
		Flow:         flow,
		Date:         now,
		SystemOrigin: systemOriginReconciliationReversal,
		SystemLocked: true,
	}
	if err := reversal.Validate(); err != nil {
		return Expense{}, err
	}
	if err := s.insertSystemExpenseTx(tx, userID, reversal); err != nil {
		return Expense{}, err
	}

	_, err = tx.Exec(`
		UPDATE reconciliations
		SET reversal_expense_id = $1, status = 'reverted', reverted_at = $2
		WHERE id = $3
	`, reversal.ID, now, rec.ID)
	if err != nil {
		return Expense{}, err
	}

	if err := tx.Commit(); err != nil {
		return Expense{}, err
	}
	return reversal, nil
}

func scanRecurringExpense(scanner interface{ Scan(...any) error }) (RecurringExpense, error) {
	var re RecurringExpense
	var tagsStr sql.NullString
	err := scanner.Scan(&re.ID, &re.Name, &re.Amount, &re.Currency, &re.Category, &re.StartDate, &re.Interval, &re.Occurrences, &re.Flow, &tagsStr)
	if err != nil {
		return RecurringExpense{}, err
	}
	if tagsStr.Valid && tagsStr.String != "" {
		if err := json.Unmarshal([]byte(tagsStr.String), &re.Tags); err != nil {
			return RecurringExpense{}, fmt.Errorf("failed to parse tags for recurring expense %s: %v", re.ID, err)
		}
	}
	return re, nil
}

func (s *databaseStore) GetRecurringExpenses(userID string) ([]RecurringExpense, error) {
	query := `SELECT id, name, amount, currency, category, start_date, interval, occurrences, flow, tags FROM recurring_expenses WHERE user_id = $1`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query recurring expenses: %v", err)
	}
	defer rows.Close()
	var recurringExpenses []RecurringExpense
	for rows.Next() {
		re, err := scanRecurringExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recurring expense: %v", err)
		}
		recurringExpenses = append(recurringExpenses, re)
	}
	return recurringExpenses, nil
}

func (s *databaseStore) GetRecurringExpense(userID, id string) (RecurringExpense, error) {
	query := `SELECT id, name, amount, currency, category, start_date, interval, occurrences, flow, tags FROM recurring_expenses WHERE user_id = $1 AND id = $2`
	re, err := scanRecurringExpense(s.db.QueryRow(query, userID, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return RecurringExpense{}, fmt.Errorf("recurring expense with ID %s not found", id)
		}
		return RecurringExpense{}, fmt.Errorf("failed to get recurring expense: %v", err)
	}
	return re, nil
}

func (s *databaseStore) AddRecurringExpense(userID string, recurringExpense RecurringExpense) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback() // Rollback on error

	if recurringExpense.ID == "" {
		recurringExpense.ID = uuid.New().String()
	}
	if recurringExpense.Flow == "" {
		if recurringExpense.Amount >= 0 {
			recurringExpense.Flow = "income"
		} else {
			recurringExpense.Flow = "expense"
		}
	}
	if recurringExpense.Currency == "" {
		if currency, err := s.GetCurrency(userID); err == nil {
			recurringExpense.Currency = currency
		}
	}
	tagsJSON, _ := json.Marshal(recurringExpense.Tags)
	ruleQuery := `
		INSERT INTO recurring_expenses (id, user_id, name, amount, currency, category, start_date, interval, occurrences, flow, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = tx.Exec(ruleQuery, recurringExpense.ID, userID, recurringExpense.Name, recurringExpense.Amount, recurringExpense.Currency, recurringExpense.Category, recurringExpense.StartDate, recurringExpense.Interval, recurringExpense.Occurrences, recurringExpense.Flow, string(tagsJSON))
	if err != nil {
		return fmt.Errorf("failed to insert recurring expense rule: %v", err)
	}

	expensesToAdd := generateExpensesFromRecurring(userID, recurringExpense, false)
	if len(expensesToAdd) > 0 {
		stmt, err := tx.Prepare(pq.CopyIn("expenses", "id", "user_id", "recurring_id", "name", "category", "amount", "currency", "date", "flow", "tags"))
		if err != nil {
			return fmt.Errorf("failed to prepare copy in: %v", err)
		}
		defer stmt.Close()
		for _, exp := range expensesToAdd {
			expTagsJSON, _ := json.Marshal(exp.Tags)
			_, err = stmt.Exec(exp.ID, exp.UserID, exp.RecurringID, exp.Name, exp.Category, exp.Amount, exp.Currency, exp.Date, exp.Flow, string(expTagsJSON))
			if err != nil {
				return fmt.Errorf("failed to execute copy in: %v", err)
			}
		}
		if _, err = stmt.Exec(); err != nil {
			return fmt.Errorf("failed to finalize copy in: %v", err)
		}
	}
	return tx.Commit()
}

func (s *databaseStore) UpdateRecurringExpense(userID, id string, recurringExpense RecurringExpense, updateAll bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()
	recurringExpense.ID = id // Ensure ID is preserved
	if recurringExpense.Flow == "" {
		if recurringExpense.Amount >= 0 {
			recurringExpense.Flow = "income"
		} else {
			recurringExpense.Flow = "expense"
		}
	}
	if recurringExpense.Currency == "" {
		if currency, err := s.GetCurrency(userID); err == nil {
			recurringExpense.Currency = currency
		}
	}
	tagsJSON, _ := json.Marshal(recurringExpense.Tags)
	ruleQuery := `
		UPDATE recurring_expenses
		SET name = $1, amount = $2, category = $3, start_date = $4, interval = $5, occurrences = $6, tags = $7, currency = $8, flow = $9
		WHERE user_id = $10 AND id = $11
	`
	res, err := tx.Exec(ruleQuery, recurringExpense.Name, recurringExpense.Amount, recurringExpense.Category, recurringExpense.StartDate, recurringExpense.Interval, recurringExpense.Occurrences, string(tagsJSON), recurringExpense.Currency, recurringExpense.Flow, userID, id)
	if err != nil {
		return fmt.Errorf("failed to update recurring expense rule: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("recurring expense with ID %s not found to update", id)
	}

	var deleteQuery string
	if updateAll {
		deleteQuery = `DELETE FROM expenses WHERE user_id = $1 AND recurring_id = $2`
		_, err = tx.Exec(deleteQuery, userID, id)
	} else {
		deleteQuery = `DELETE FROM expenses WHERE user_id = $1 AND recurring_id = $2 AND date > $3`
		_, err = tx.Exec(deleteQuery, userID, id, time.Now())
	}
	if err != nil {
		return fmt.Errorf("failed to delete old expense instances for update: %v", err)
	}

	expensesToAdd := generateExpensesFromRecurring(userID, recurringExpense, !updateAll)
	if len(expensesToAdd) > 0 {
		stmt, err := tx.Prepare(pq.CopyIn("expenses", "id", "user_id", "recurring_id", "name", "category", "amount", "currency", "date", "flow", "tags"))
		if err != nil {
			return fmt.Errorf("failed to prepare copy in for update: %v", err)
		}
		defer stmt.Close()
		for _, exp := range expensesToAdd {
			expTagsJSON, _ := json.Marshal(exp.Tags)
			_, err = stmt.Exec(exp.ID, exp.UserID, exp.RecurringID, exp.Name, exp.Category, exp.Amount, exp.Currency, exp.Date, exp.Flow, string(expTagsJSON))
			if err != nil {
				return fmt.Errorf("failed to execute copy in for update: %v", err)
			}
		}
		if _, err = stmt.Exec(); err != nil {
			return fmt.Errorf("failed to finalize copy in for update: %v", err)
		}
	}
	return tx.Commit()
}

func (s *databaseStore) RemoveRecurringExpense(userID, id string, removeAll bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM recurring_expenses WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return fmt.Errorf("failed to delete recurring expense rule: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("recurring expense with ID %s not found", id)
	}

	var deleteQuery string
	if removeAll {
		deleteQuery = `DELETE FROM expenses WHERE user_id = $1 AND recurring_id = $2`
		_, err = tx.Exec(deleteQuery, userID, id)
	} else {
		deleteQuery = `DELETE FROM expenses WHERE user_id = $1 AND recurring_id = $2 AND date > $3`
		_, err = tx.Exec(deleteQuery, userID, id, time.Now())
	}
	if err != nil {
		return fmt.Errorf("failed to delete expense instances: %v", err)
	}
	return tx.Commit()
}

func generateExpensesFromRecurring(userID string, recExp RecurringExpense, fromToday bool) []Expense {
	var expenses []Expense
	currentDate := recExp.StartDate
	today := time.Now()
	flow := recExp.Flow
	if flow == "" {
		if recExp.Amount >= 0 {
			flow = "income"
		} else {
			flow = "expense"
		}
	}
	occurrencesToGenerate := recExp.Occurrences
	if fromToday {
		for currentDate.Before(today) && (recExp.Occurrences == 0 || occurrencesToGenerate > 0) {
			switch recExp.Interval {
			case "daily":
				currentDate = currentDate.AddDate(0, 0, 1)
			case "weekly":
				currentDate = currentDate.AddDate(0, 0, 7)
			case "monthly":
				currentDate = currentDate.AddDate(0, 1, 0)
			case "yearly":
				currentDate = currentDate.AddDate(1, 0, 0)
			default:
				return expenses // Stop if interval is invalid
			}
			if recExp.Occurrences > 0 {
				occurrencesToGenerate--
			}
		}
	}
	limit := occurrencesToGenerate
	// if recExp.Occurrences == 0 {
	// 	limit = 2000 // Heuristic for "indefinite"
	// }

	for range limit {
		expense := Expense{
			UserID:      userID,
			Flow:        flow,
			ID:          uuid.New().String(),
			RecurringID: recExp.ID,
			Name:        recExp.Name,
			Category:    recExp.Category,
			Amount:      recExp.Amount,
			Currency:    recExp.Currency,
			Date:        currentDate,
			Tags:        recExp.Tags,
		}
		expenses = append(expenses, expense)
		switch recExp.Interval {
		case "daily":
			currentDate = currentDate.AddDate(0, 0, 1)
		case "weekly":
			currentDate = currentDate.AddDate(0, 0, 7)
		case "monthly":
			currentDate = currentDate.AddDate(0, 1, 0)
		case "yearly":
			currentDate = currentDate.AddDate(1, 0, 0)
		default:
			return expenses
		}
	}
	return expenses
}
