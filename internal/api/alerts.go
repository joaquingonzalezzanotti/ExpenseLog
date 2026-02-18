package api

import (
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
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
}

type liquidityAlertsResponse struct {
	Currency         string               `json:"currency"`
	WindowDays       int                  `json:"windowDays"`
	BalanceNow       float64              `json:"balanceNow"`
	ProjectedBalance float64              `json:"projectedBalance"`
	AlertCount       int                  `json:"alertCount"`
	Alerts           []liquidityAlertItem `json:"alerts"`
}

type recurringProjection struct {
	ID       string
	Name     string
	Category string
	DueDate  time.Time
	Amount   float64
}

func isCashAccountSource(source string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(source))
	return normalized == "" || normalized == "CA"
}

func daysUntilDate(now, due time.Time) int {
	nowLocal := now.In(time.Local)
	dueLocal := due.In(time.Local)
	startNow := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.Local)
	startDue := time.Date(dueLocal.Year(), dueLocal.Month(), dueLocal.Day(), 0, 0, 0, 0, time.Local)
	return int(startDue.Sub(startNow).Hours() / 24)
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
		currency = "ars"
	}
	days := 4
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		parsed, err := strconv.Atoi(q)
		if err != nil || parsed < 0 || parsed > 30 {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid days value"})
			return
		}
		days = parsed
	}

	now := time.Now()
	horizon := now.AddDate(0, 0, days)

	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load expenses"})
		log.Printf("API ERROR: Failed to load expenses for liquidity alerts: %v\n", err)
		return
	}
	balance := 0.0
	for _, exp := range expenses {
		if strings.ToLower(strings.TrimSpace(exp.Currency)) != currency {
			continue
		}
		if !isCashAccountSource(exp.Source) {
			continue
		}
		if exp.Date.After(now) {
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
		if !exp.Date.After(now) || exp.Date.After(horizon) {
			continue
		}
		timeline = append(timeline, recurringProjection{
			ID:       exp.RecurringID,
			Name:     exp.Name,
			Category: exp.Category,
			DueDate:  exp.Date,
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

	projectedBalance := balance
	var alerts []liquidityAlertItem
	for _, item := range timeline {
		balanceBefore := projectedBalance
		projectedBalance += item.Amount
		if item.Amount >= 0 {
			continue
		}
		required := math.Abs(item.Amount)
		if balanceBefore >= required {
			continue
		}
		shortfall := required - balanceBefore
		if shortfall < 0 {
			shortfall = 0
		}
		alerts = append(alerts, liquidityAlertItem{
			RecurringID:    item.ID,
			Name:           item.Name,
			Category:       item.Category,
			DueDate:        item.DueDate,
			DaysUntil:      daysUntilDate(now, item.DueDate),
			RequiredAmount: required,
			BalanceBefore:  balanceBefore,
			BalanceAfter:   projectedBalance,
			Shortfall:      shortfall,
		})
	}

	resp := liquidityAlertsResponse{
		Currency:         currency,
		WindowDays:       days,
		BalanceNow:       balance,
		ProjectedBalance: projectedBalance,
		AlertCount:       len(alerts),
		Alerts:           alerts,
	}
	writeJSON(w, http.StatusOK, resp)
}
