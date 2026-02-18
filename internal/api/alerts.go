package api

import (
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tanq16/expenseowl/internal/storage"
)

type liquidityAlertItem struct {
	RecurringID    string    `json:"recurringId"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	DueDate        time.Time `json:"dueDate"`
	DaysUntil      int       `json:"daysUntil"`
	RequiredAmount float64   `json:"requiredAmount"`
	BalanceBefore  float64   `json:"balanceBefore"`
	BalanceAfter   float64   `json:"balanceAfter"`
	Shortfall      float64   `json:"shortfall"`
	Severity       string    `json:"severity"` // info | critical
	Kind           string    `json:"kind"`     // preview_7d | monitor_4d | risk_4d | due
}

type liquidityAlertsResponse struct {
	Currency         string               `json:"currency"`
	WindowDays       int                  `json:"windowDays"`
	BalanceNow       float64              `json:"balanceNow"`
	ProjectedBalance float64              `json:"projectedBalance"`
	AlertCount       int                  `json:"alertCount"`
	CriticalCount    int                  `json:"criticalCount"`
	InfoCount        int                  `json:"infoCount"`
	Alerts           []liquidityAlertItem `json:"alerts"`
}

type recurringProjection struct {
	ID       string
	Name     string
	Category string
	DueDate  time.Time
	Amount   float64
}

const (
	liquidityDefaultCurrency    = "ars"
	liquidityDefaultWindowDays  = 7
	liquidityCriticalWindowDays = 4
	liquidityRecentDueDays      = 1
	liquidityMaxWindowDays      = 30
)

func isCashAccountSource(source string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(source))
	return normalized == "" || normalized == "CA"
}

func localDayStart(t time.Time) time.Time {
	local := t.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
}

// Recurring due dates are treated as calendar-day events (not timestamp events).
// We take UTC Y-M-D as the authoritative date and map it to local midnight.
func recurringDueLocalDay(due time.Time) time.Time {
	utc := due.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.Local)
}

func daysUntilDate(now, due time.Time) int {
	startNow := localDayStart(now)
	startDue := recurringDueLocalDay(due)
	return int(startDue.Sub(startNow).Hours() / 24)
}

func computeLiquidityAlerts(expenses []storage.Expense, currency string, days int, now time.Time) liquidityAlertsResponse {
	todayStart := localDayStart(now)
	horizonDay := todayStart.AddDate(0, 0, days)
	recentCutoffDay := todayStart.AddDate(0, 0, -liquidityRecentDueDays)

	balance := 0.0
	for _, exp := range expenses {
		if strings.ToLower(strings.TrimSpace(exp.Currency)) != currency {
			continue
		}
		if !isCashAccountSource(exp.Source) {
			continue
		}
		if strings.TrimSpace(exp.RecurringID) != "" {
			if recurringDueLocalDay(exp.Date).After(todayStart) {
				continue
			}
		} else if exp.Date.After(now) {
			continue
		}
		balance += exp.Amount
	}

	var timeline []recurringProjection
	for _, exp := range expenses {
		if strings.ToLower(strings.TrimSpace(exp.Currency)) != currency {
			continue
		}
		if !isCashAccountSource(exp.Source) {
			continue
		}
		if strings.TrimSpace(exp.RecurringID) == "" {
			continue
		}
		dueDay := recurringDueLocalDay(exp.Date)
		if !dueDay.After(todayStart) || dueDay.After(horizonDay) {
			continue
		}
		timeline = append(timeline, recurringProjection{
			ID:       exp.RecurringID,
			Name:     exp.Name,
			Category: exp.Category,
			DueDate:  dueDay,
			Amount:   exp.Amount,
		})
	}

	// Recently due recurring expenses: always informational (never critical).
	historyBalance := 0.0
	for _, exp := range expenses {
		if strings.ToLower(strings.TrimSpace(exp.Currency)) != currency {
			continue
		}
		if !isCashAccountSource(exp.Source) {
			continue
		}
		if strings.TrimSpace(exp.RecurringID) != "" {
			if recurringDueLocalDay(exp.Date).After(recentCutoffDay) {
				continue
			}
		} else if exp.Date.After(recentCutoffDay) {
			continue
		}
		historyBalance += exp.Amount
	}
	var recentDueTimeline []recurringProjection
	for _, exp := range expenses {
		if strings.ToLower(strings.TrimSpace(exp.Currency)) != currency {
			continue
		}
		if !isCashAccountSource(exp.Source) {
			continue
		}
		if strings.TrimSpace(exp.RecurringID) == "" {
			continue
		}
		dueDay := recurringDueLocalDay(exp.Date)
		if dueDay.Before(recentCutoffDay) || dueDay.After(todayStart) {
			continue
		}
		recentDueTimeline = append(recentDueTimeline, recurringProjection{
			ID:       exp.RecurringID,
			Name:     exp.Name,
			Category: exp.Category,
			DueDate:  dueDay,
			Amount:   exp.Amount,
		})
	}
	sort.Slice(timeline, func(i, j int) bool {
		if timeline[i].DueDate.Equal(timeline[j].DueDate) {
			if timeline[i].Name == timeline[j].Name {
				return timeline[i].ID < timeline[j].ID
			}
			return timeline[i].Name < timeline[j].Name
		}
		return timeline[i].DueDate.Before(timeline[j].DueDate)
	})
	sort.Slice(recentDueTimeline, func(i, j int) bool {
		if recentDueTimeline[i].DueDate.Equal(recentDueTimeline[j].DueDate) {
			if recentDueTimeline[i].Name == recentDueTimeline[j].Name {
				return recentDueTimeline[i].ID < recentDueTimeline[j].ID
			}
			return recentDueTimeline[i].Name < recentDueTimeline[j].Name
		}
		return recentDueTimeline[i].DueDate.Before(recentDueTimeline[j].DueDate)
	})

	projectedBalance := balance
	var alerts []liquidityAlertItem
	rollingRecentBalance := historyBalance
	for _, item := range recentDueTimeline {
		balanceBefore := rollingRecentBalance
		rollingRecentBalance += item.Amount
		if item.Amount >= 0 {
			continue
		}
		required := math.Abs(item.Amount)
		shortfall := 0.0
		if rollingRecentBalance < 0 {
			shortfall = math.Abs(rollingRecentBalance)
		}
		alerts = append(alerts, liquidityAlertItem{
			RecurringID:    item.ID,
			Name:           item.Name,
			Category:       item.Category,
			DueDate:        item.DueDate,
			DaysUntil:      daysUntilDate(now, item.DueDate),
			RequiredAmount: required,
			BalanceBefore:  balanceBefore,
			BalanceAfter:   rollingRecentBalance,
			Shortfall:      shortfall,
			Severity:       "info",
			Kind:           "due",
		})
	}

	for _, item := range timeline {
		balanceBefore := projectedBalance
		projectedBalance += item.Amount
		if item.Amount >= 0 {
			continue
		}
		required := math.Abs(item.Amount)
		daysUntil := daysUntilDate(now, item.DueDate)
		shortfall := 0.0
		if projectedBalance < 0 {
			shortfall = math.Abs(projectedBalance)
		}
		severity := "info"
		kind := "preview_7d"
		if daysUntil <= liquidityCriticalWindowDays {
			kind = "monitor_4d"
			if projectedBalance < 0 {
				severity = "critical"
				kind = "risk_4d"
			}
		}
		alerts = append(alerts, liquidityAlertItem{
			RecurringID:    item.ID,
			Name:           item.Name,
			Category:       item.Category,
			DueDate:        item.DueDate,
			DaysUntil:      daysUntil,
			RequiredAmount: required,
			BalanceBefore:  balanceBefore,
			BalanceAfter:   projectedBalance,
			Shortfall:      shortfall,
			Severity:       severity,
			Kind:           kind,
		})
	}

	sort.Slice(alerts, func(i, j int) bool {
		leftCritical := alerts[i].Severity == "critical"
		rightCritical := alerts[j].Severity == "critical"
		if leftCritical != rightCritical {
			return leftCritical
		}
		if alerts[i].DueDate.Equal(alerts[j].DueDate) {
			return alerts[i].Name < alerts[j].Name
		}
		return alerts[i].DueDate.Before(alerts[j].DueDate)
	})

	criticalCount := 0
	for _, item := range alerts {
		if item.Severity == "critical" {
			criticalCount++
		}
	}

	return liquidityAlertsResponse{
		Currency:         currency,
		WindowDays:       days,
		BalanceNow:       balance,
		ProjectedBalance: projectedBalance,
		AlertCount:       len(alerts),
		CriticalCount:    criticalCount,
		InfoCount:        len(alerts) - criticalCount,
		Alerts:           alerts,
	}
}

func (h *Handler) GetLiquidityAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	currency := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("currency")))
	if currency == "" {
		currency = liquidityDefaultCurrency
	}
	days := liquidityDefaultWindowDays
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		parsed, err := strconv.Atoi(q)
		if err != nil || parsed < 0 || parsed > liquidityMaxWindowDays {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid days value"})
			return
		}
		days = parsed
	}
	now := time.Now()

	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load expenses"})
		log.Printf("API ERROR: Failed to load expenses for liquidity alerts: %v\n", err)
		return
	}
	resp := computeLiquidityAlerts(expenses, currency, days, now)
	writeJSON(w, http.StatusOK, resp)
}
