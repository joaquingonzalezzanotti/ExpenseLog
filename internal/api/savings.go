package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
)

type savingsGoalPayload struct {
	Name         string  `json:"name"`
	TargetAmount float64 `json:"targetAmount"`
	Currency     string  `json:"currency"`
	TargetDate   string  `json:"targetDate"`
	Status       string  `json:"status"`
}

type savingsAllocationPayload struct {
	Amount float64 `json:"amount"`
	Note   string  `json:"note"`
	Date   string  `json:"date"`
}

func parseOptionalDate(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func (h *Handler) HandleSavingsGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		goals, err := h.storage.GetSavingsGoals(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to list savings goals"})
			return
		}
		writeJSON(w, http.StatusOK, goals)
	case http.MethodPost:
		var payload savingsGoalPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
			return
		}
		targetDate, err := parseOptionalDate(payload.TargetDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid targetDate format"})
			return
		}
		goal, err := h.storage.CreateSavingsGoal(userID, storage.SavingsGoal{
			Name:         payload.Name,
			TargetAmount: payload.TargetAmount,
			Currency:     payload.Currency,
			TargetDate:   targetDate,
			Status:       payload.Status,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, goal)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
	}
}

func (h *Handler) HandleSavingsGoalActions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api"), "/savings/goals/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Goal ID is required"})
		return
	}
	goalID := strings.TrimSpace(segments[0])

	if len(segments) == 1 {
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
			return
		}
		var payload savingsGoalPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
			return
		}
		targetDate, err := parseOptionalDate(payload.TargetDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid targetDate format"})
			return
		}
		goal, err := h.storage.UpdateSavingsGoal(userID, goalID, storage.SavingsGoalUpdateInput{
			Name:         payload.Name,
			TargetAmount: payload.TargetAmount,
			TargetDate:   targetDate,
			Status:       payload.Status,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, goal)
		return
	}

	if len(segments) != 2 || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(segments[1]))
	if action != "contribute" && action != "withdraw" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Not found"})
		return
	}

	var payload savingsAllocationPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	at, err := parseOptionalDate(payload.Date)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid date format"})
		return
	}
	movementType := "contribution"
	if action == "withdraw" {
		movementType = "withdrawal"
	}
	allocation, err := h.storage.AddSavingsAllocation(userID, goalID, storage.SavingsAllocation{
		Type:   movementType,
		Amount: payload.Amount,
		Note:   payload.Note,
		Date:   at,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, allocation)
}

func (h *Handler) GetSavingsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	summary, err := h.storage.GetSavingsSummary(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get savings summary"})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
