package storage

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func normalizeSavingsStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "completed"
	case "archived":
		return "archived"
	default:
		return "active"
	}
}

func normalizeSavingsAllocationType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "withdrawal":
		return "withdrawal"
	case "adjustment":
		return "adjustment"
	default:
		return "contribution"
	}
}

func (s *databaseStore) GetSavingsGoals(userID string) ([]SavingsGoal, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, name, target_amount, currency, target_date, status, created_at, updated_at
		FROM savings_goals
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query savings goals: %v", err)
	}
	defer rows.Close()

	goals := make([]SavingsGoal, 0)
	for rows.Next() {
		var goal SavingsGoal
		var targetDate sql.NullTime
		if err := rows.Scan(&goal.ID, &goal.UserID, &goal.Name, &goal.TargetAmount, &goal.Currency, &targetDate, &goal.Status, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan savings goal: %v", err)
		}
		goal.Currency = normalizeCurrencyCode(goal.Currency)
		goal.Status = normalizeSavingsStatus(goal.Status)
		if targetDate.Valid {
			goal.TargetDate = targetDate.Time
		}
		goals = append(goals, goal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed while iterating savings goals: %v", err)
	}
	return goals, nil
}

func (s *databaseStore) CreateSavingsGoal(userID string, goal SavingsGoal) (SavingsGoal, error) {
	now := time.Now().UTC()
	created := SavingsGoal{
		ID:           uuid.New().String(),
		UserID:       userID,
		Name:         strings.TrimSpace(goal.Name),
		TargetAmount: math.Abs(goal.TargetAmount),
		Currency:     normalizeCurrencyCode(goal.Currency),
		TargetDate:   goal.TargetDate,
		Status:       normalizeSavingsStatus(goal.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if created.Name == "" {
		return SavingsGoal{}, fmt.Errorf("goal name is required")
	}
	if created.TargetAmount <= 0 {
		return SavingsGoal{}, fmt.Errorf("target amount must be greater than zero")
	}

	var targetDate any
	if !created.TargetDate.IsZero() {
		targetDate = created.TargetDate
	}

	_, err := s.db.Exec(`
		INSERT INTO savings_goals (id, user_id, name, target_amount, currency, target_date, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, created.ID, created.UserID, created.Name, created.TargetAmount, created.Currency, targetDate, created.Status, created.CreatedAt, created.UpdatedAt)
	if err != nil {
		return SavingsGoal{}, fmt.Errorf("failed to create savings goal: %v", err)
	}
	return created, nil
}

func (s *databaseStore) UpdateSavingsGoal(userID, id string, input SavingsGoalUpdateInput) (SavingsGoal, error) {
	name := strings.TrimSpace(input.Name)
	status := normalizeSavingsStatus(input.Status)
	targetAmount := math.Abs(input.TargetAmount)
	if name == "" {
		return SavingsGoal{}, fmt.Errorf("goal name is required")
	}
	if targetAmount <= 0 {
		return SavingsGoal{}, fmt.Errorf("target amount must be greater than zero")
	}
	var targetDate any
	if !input.TargetDate.IsZero() {
		targetDate = input.TargetDate
	}
	if _, err := s.db.Exec(`
		UPDATE savings_goals
		SET name = $1,
		    target_amount = $2,
		    target_date = $3,
		    status = $4,
		    updated_at = NOW()
		WHERE user_id = $5 AND id = $6
	`, name, targetAmount, targetDate, status, userID, id); err != nil {
		return SavingsGoal{}, fmt.Errorf("failed to update savings goal: %v", err)
	}

	var goal SavingsGoal
	var targetDateOut sql.NullTime
	if err := s.db.QueryRow(`
		SELECT id, user_id, name, target_amount, currency, target_date, status, created_at, updated_at
		FROM savings_goals
		WHERE user_id = $1 AND id = $2
	`, userID, id).Scan(&goal.ID, &goal.UserID, &goal.Name, &goal.TargetAmount, &goal.Currency, &targetDateOut, &goal.Status, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return SavingsGoal{}, fmt.Errorf("savings goal not found")
		}
		return SavingsGoal{}, fmt.Errorf("failed to fetch updated savings goal: %v", err)
	}
	if targetDateOut.Valid {
		goal.TargetDate = targetDateOut.Time
	}
	goal.Currency = normalizeCurrencyCode(goal.Currency)
	goal.Status = normalizeSavingsStatus(goal.Status)
	return goal, nil
}

func (s *databaseStore) AddSavingsAllocation(userID, goalID string, allocation SavingsAllocation) (SavingsAllocation, error) {
	kind := normalizeSavingsAllocationType(allocation.Type)
	amount := math.Abs(allocation.Amount)
	if amount <= 0 {
		return SavingsAllocation{}, fmt.Errorf("amount must be greater than zero")
	}
	note := strings.TrimSpace(allocation.Note)
	at := allocation.Date
	if at.IsZero() {
		at = time.Now().UTC()
	}

	var goalCurrency string
	if err := s.db.QueryRow(`SELECT currency FROM savings_goals WHERE user_id = $1 AND id = $2`, userID, goalID).Scan(&goalCurrency); err != nil {
		if err == sql.ErrNoRows {
			return SavingsAllocation{}, fmt.Errorf("savings goal not found")
		}
		return SavingsAllocation{}, fmt.Errorf("failed to validate savings goal: %v", err)
	}
	goalCurrency = normalizeCurrencyCode(goalCurrency)

	created := SavingsAllocation{
		ID:        uuid.New().String(),
		UserID:    userID,
		GoalID:    goalID,
		Type:      kind,
		Amount:    amount,
		Currency:  goalCurrency,
		Note:      note,
		Date:      at.UTC(),
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(`
		INSERT INTO savings_allocations (id, user_id, goal_id, type, amount, currency, note, date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, created.ID, created.UserID, created.GoalID, created.Type, created.Amount, created.Currency, created.Note, created.Date, created.CreatedAt)
	if err != nil {
		return SavingsAllocation{}, fmt.Errorf("failed to save savings allocation: %v", err)
	}
	return created, nil
}

func (s *databaseStore) GetSavingsSummary(userID string) (SavingsSummary, error) {
	summary := SavingsSummary{
		TotalReservedByCurrency: make(map[string]float64),
		SecondaryCurrencyFunds:  make(map[string]float64),
		Goals:                   make([]SavingsGoalSnapshot, 0),
	}
	goals, err := s.GetSavingsGoals(userID)
	if err != nil {
		return summary, err
	}

	for _, goal := range goals {
		var reserved float64
		if err := s.db.QueryRow(`
			SELECT COALESCE(SUM(
				CASE
					WHEN type = 'withdrawal' THEN -ABS(amount)
					ELSE ABS(amount)
				END
			), 0)
			FROM savings_allocations
			WHERE user_id = $1 AND goal_id = $2
		`, userID, goal.ID).Scan(&reserved); err != nil {
			return summary, fmt.Errorf("failed to aggregate savings allocations: %v", err)
		}
		rows, err := s.db.Query(`
			SELECT id, user_id, goal_id, type, amount, currency, COALESCE(note, ''), date, created_at
			FROM savings_allocations
			WHERE user_id = $1 AND goal_id = $2
			ORDER BY date DESC, created_at DESC
			LIMIT 10
		`, userID, goal.ID)
		if err != nil {
			return summary, fmt.Errorf("failed to read savings allocations: %v", err)
		}

		allocations := make([]SavingsAllocation, 0)
		for rows.Next() {
			var movement SavingsAllocation
			if err := rows.Scan(&movement.ID, &movement.UserID, &movement.GoalID, &movement.Type, &movement.Amount, &movement.Currency, &movement.Note, &movement.Date, &movement.CreatedAt); err != nil {
				rows.Close()
				return summary, fmt.Errorf("failed to scan savings allocation: %v", err)
			}
			movement.Currency = normalizeCurrencyCode(movement.Currency)
			movement.Type = normalizeSavingsAllocationType(movement.Type)
			allocations = append(allocations, movement)
		}
		rows.Close()

		if reserved < 0 {
			reserved = 0
		}
		summary.TotalReservedByCurrency[goal.Currency] += reserved
		remaining := goal.TargetAmount - reserved
		if remaining < 0 {
			remaining = 0
		}
		progress := 0.0
		if goal.TargetAmount > 0 {
			progress = reserved / goal.TargetAmount
			if progress > 1 {
				progress = 1
			}
		}

		monthlySuggestion := 0.0
		if !goal.TargetDate.IsZero() && remaining > 0 {
			monthsLeft := int(math.Ceil(goal.TargetDate.Sub(time.Now().UTC()).Hours() / (24 * 30)))
			if monthsLeft < 1 {
				monthsLeft = 1
			}
			monthlySuggestion = remaining / float64(monthsLeft)
		}

		summary.Goals = append(summary.Goals, SavingsGoalSnapshot{
			Goal:              goal,
			ReservedAmount:    reserved,
			ProgressRatio:     progress,
			RemainingAmount:   remaining,
			MonthlySuggestion: monthlySuggestion,
			Allocations:       allocations,
		})
	}

	baseCurrency, err := s.GetCurrency(userID)
	if err != nil {
		baseCurrency = "ars"
	}
	baseCurrency = normalizeCurrencyCode(baseCurrency)

	rows, err := s.db.Query(`
		SELECT LOWER(COALESCE(currency, '')), COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE user_id = $1
		GROUP BY LOWER(COALESCE(currency, ''))
	`, userID)
	if err != nil {
		return summary, fmt.Errorf("failed to compute secondary currency balances: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var currency string
		var amount float64
		if err := rows.Scan(&currency, &amount); err != nil {
			return summary, fmt.Errorf("failed to scan secondary currency balance: %v", err)
		}
		currency = normalizeCurrencyCode(currency)
		if currency == baseCurrency {
			continue
		}
		if amount <= 0 {
			continue
		}
		summary.SecondaryCurrencyFunds[currency] = amount
	}

	sort.Slice(summary.Goals, func(i, j int) bool {
		return summary.Goals[i].Goal.CreatedAt.After(summary.Goals[j].Goal.CreatedAt)
	})
	return summary, nil
}
