package api

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/web"
)

// Handler holds the storage interface
type Handler struct {
	storage storage.Storage
}

// NewHandler creates a new API handler
func NewHandler(s storage.Storage) *Handler {
	return &Handler{
		storage: s,
	}
}

// ErrorResponse is a generic JSON error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func requireUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := userIDFromContext(r.Context())
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return "", false
	}
	return userID, true
}

// ------------------------------------------------------------
// Config Handlers
// ------------------------------------------------------------

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	config, err := h.storage.GetConfig(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get config"})
		log.Printf("API ERROR: Failed to get config: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	categories, err := h.storage.GetCategories(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get categories"})
		log.Printf("API ERROR: Failed to get categories: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) UpdateCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var categories []string
	if err := json.NewDecoder(r.Body).Decode(&categories); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	// Validate that we have at least one category
	if len(categories) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "At least one category is required"})
		return
	}

	var sanitizedCategories []string
	for _, category := range categories {
		sanitized, err := storage.ValidateCategory(category)
		if err != nil {
			log.Printf("API ERROR: Invalid category provided: %v\n", err)
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("Invalid category '%s': %v", category, err)})
			return
		}
		sanitizedCategories = append(sanitizedCategories, sanitized)
	}

	if err := h.storage.UpdateCategories(userID, sanitizedCategories); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update categories"})
		log.Printf("API ERROR: Failed to update categories: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

type categoryPayload struct {
	Name string `json:"name"`
}

type categoryRenamePayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func findCategoryIndex(categories []string, name string) int {
	for i, category := range categories {
		if strings.EqualFold(category, name) {
			return i
		}
	}
	return -1
}

func (h *Handler) AddCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var payload categoryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	name, err := storage.ValidateCategory(payload.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	categories, err := h.storage.GetCategories(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get categories"})
		log.Printf("API ERROR: Failed to get categories: %v\n", err)
		return
	}
	if findCategoryIndex(categories, name) != -1 {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Category already exists"})
		return
	}
	updated := append(categories, name)
	if err := h.storage.UpdateCategories(userID, updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update categories"})
		log.Printf("API ERROR: Failed to update categories: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) RenameCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var payload categoryRenamePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	from, err := storage.ValidateCategory(payload.From)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid current category"})
		return
	}
	to, err := storage.ValidateCategory(payload.To)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	categories, err := h.storage.GetCategories(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get categories"})
		log.Printf("API ERROR: Failed to get categories: %v\n", err)
		return
	}
	index := findCategoryIndex(categories, from)
	if index == -1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Category not found"})
		return
	}
	for i, category := range categories {
		if i != index && strings.EqualFold(category, to) {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Category already exists"})
			return
		}
	}
	if strings.EqualFold(categories[index], to) {
		writeJSON(w, http.StatusOK, categories)
		return
	}
	categories[index] = to
	if err := h.storage.UpdateCategories(userID, categories); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update categories"})
		log.Printf("API ERROR: Failed to update categories: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var payload categoryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	name, err := storage.ValidateCategory(payload.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	categories, err := h.storage.GetCategories(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get categories"})
		log.Printf("API ERROR: Failed to get categories: %v\n", err)
		return
	}
	index := findCategoryIndex(categories, name)
	if index == -1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Category not found"})
		return
	}
	if len(categories) <= 1 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "At least one category is required"})
		return
	}
	updated := append(categories[:index], categories[index+1:]...)
	if err := h.storage.UpdateCategories(userID, updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update categories"})
		log.Printf("API ERROR: Failed to update categories: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) GetCurrency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	currency, err := h.storage.GetCurrency(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get currency"})
		log.Printf("API ERROR: Failed to get currency: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, currency)
}

func (h *Handler) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var currency string
	if err := json.NewDecoder(r.Body).Decode(&currency); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := h.storage.UpdateCurrency(userID, currency); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		log.Printf("API ERROR: Failed to update currency: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *Handler) GetStartDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	startDate, err := h.storage.GetStartDate(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get start date"})
		log.Printf("API ERROR: Failed to get start date: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, startDate)
}

func (h *Handler) UpdateStartDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var startDate int
	if err := json.NewDecoder(r.Body).Decode(&startDate); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := h.storage.UpdateStartDate(userID, startDate); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		log.Printf("API ERROR: Failed to update start date: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// ------------------------------------------------------------
// Expense Handlers
// ------------------------------------------------------------

func (h *Handler) AddExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "expense:add:"+userID)
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	var expense storage.Expense
	if err := json.NewDecoder(r.Body).Decode(&expense); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := expense.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, adjustedAmount, err := normalizeFlow(expense.Flow, expense.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	expense.Flow = flow
	expense.Amount = adjustedAmount
	if expense.Currency == "" {
		if cfgCur, err := h.storage.GetCurrency(userID); err == nil {
			expense.Currency = cfgCur
		}
	}
	if expense.Date.IsZero() {
		expense.Date = time.Now()
	}
	// Generic add endpoint can never create system-generated movements.
	expense.SystemOrigin = ""
	expense.SystemLocked = false
	if err := h.storage.AddExpense(userID, expense); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to save expense"})
		log.Printf("API ERROR: Failed to save expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, expense)
}

func (h *Handler) GetExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve expenses"})
		log.Printf("API ERROR: Failed to retrieve expenses: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, expenses)
}

func (h *Handler) EditExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "expense:edit:"+userID+":"+id)
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	var expense storage.Expense
	if err := json.NewDecoder(r.Body).Decode(&expense); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	// Preserve recurring linkage for generated instances even if UI does not send recurringID.
	existingExpense, err := h.storage.GetExpense(userID, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Expense not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to load existing expense"})
		log.Printf("API ERROR: Failed to load expense before edit: %v\n", err)
		return
	}
	expense.RecurringID = existingExpense.RecurringID
	if err := expense.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, adjustedAmount, err := normalizeFlow(expense.Flow, expense.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	expense.Flow = flow
	expense.Amount = adjustedAmount
	if expense.Currency == "" {
		if cfgCur, err := h.storage.GetCurrency(userID); err == nil {
			expense.Currency = cfgCur
		}
	}
	if err := h.storage.UpdateExpense(userID, id, expense); err != nil {
		if err == storage.ErrSystemLockedExpense {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Este movimiento es de sistema. Reverti la conciliacion desde Ajustes."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to edit expense"})
		log.Printf("API ERROR: Failed to edit expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, expense)
}

func (h *Handler) DeleteExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "expense:delete:"+userID+":"+id)
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	if err := h.storage.RemoveExpense(userID, id); err != nil {
		if err == storage.ErrSystemLockedExpense {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Este movimiento es de sistema. Reverti la conciliacion desde Ajustes."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete expense"})
		log.Printf("API ERROR: Failed to delete expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *Handler) DeleteMultipleExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "expense:delete_multiple:"+userID)
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := h.storage.RemoveMultipleExpenses(userID, payload.IDs); err != nil {
		if err == storage.ErrSystemLockedExpense {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Hay movimientos de sistema seleccionados. Reverti la conciliacion desde Ajustes."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete multiple expenses"})
		log.Printf("API ERROR: Failed to delete multiple expenses: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// ------------------------------------------------------------
// Recurring Expense Handlers
// ------------------------------------------------------------

func (h *Handler) AddRecurringExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "recurring:add:"+userID)
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	var re storage.RecurringExpense
	if err := json.NewDecoder(r.Body).Decode(&re); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := re.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, adjustedAmount, err := normalizeFlow(re.Flow, re.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	re.Flow = flow
	re.Amount = adjustedAmount
	if err := h.storage.AddRecurringExpense(userID, re); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to add recurring expense"})
		log.Printf("API ERROR: Failed to add recurring expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusCreated, re)
}

func (h *Handler) GetRecurringExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	res, err := h.storage.GetRecurringExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get recurring expenses"})
		log.Printf("API ERROR: Failed to get recurring expenses: %v\n", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) UpdateRecurringExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}
	updateAll, _ := strconv.ParseBool(r.URL.Query().Get("updateAll"))
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "recurring:update:"+userID+":"+id+":"+strconv.FormatBool(updateAll))
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	var re storage.RecurringExpense
	if err := json.NewDecoder(r.Body).Decode(&re); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if err := re.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	flow, adjustedAmount, err := normalizeFlow(re.Flow, re.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	re.Flow = flow
	re.Amount = adjustedAmount
	if err := h.storage.UpdateRecurringExpense(userID, id, re, updateAll); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update recurring expense"})
		log.Printf("API ERROR: Failed to update recurring expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *Handler) DeleteRecurringExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ID parameter is required"})
		return
	}
	removeAll, _ := strconv.ParseBool(r.URL.Query().Get("removeAll"))
	releaseIdempotency, duplicate, idempoErr := beginMutationIdempotency(r, "recurring:delete:"+userID+":"+id+":"+strconv.FormatBool(removeAll))
	if idempoErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid Idempotency-Key"})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "Duplicate request"})
		return
	}
	idempotencySuccess := false
	defer releaseIdempotency(idempotencySuccess)

	if err := h.storage.RemoveRecurringExpense(userID, id, removeAll); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete recurring expense"})
		log.Printf("API ERROR: Failed to delete recurring expense: %v\n", err)
		return
	}
	idempotencySuccess = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// ------------------------------------------------------------
// Static and UI Handlers
// ------------------------------------------------------------

func (h *Handler) ServeTableView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.ServeTemplate(w, "table.html"); err != nil {
		http.Error(w, "Failed to serve template", http.StatusInternalServerError)
	}
}

func (h *Handler) ServeAnalyticsView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.ServeTemplate(w, "analisis.html"); err != nil {
		http.Error(w, "Failed to serve template", http.StatusInternalServerError)
	}
}

func (h *Handler) ServeSettingsPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "profile")
}

func (h *Handler) ServeSettingsCategoriesPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "categories")
}

func (h *Handler) ServeSettingsRecurringPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "recurring")
}

func (h *Handler) ServeSettingsReconciliationPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "reconciliation")
}

func (h *Handler) ServeSettingsReportsPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "reports")
}

func (h *Handler) ServeSettingsTelegramPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "telegram")
}

func (h *Handler) ServeSettingsWhatsAppPage(w http.ResponseWriter, r *http.Request) {
	h.serveSettingsPageWithSection(w, r, "whatsapp")
}

func (h *Handler) serveSettingsPageWithSection(w http.ResponseWriter, r *http.Request, _ string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.ServeTemplate(w, "settings.html"); err != nil {
		http.Error(w, "Failed to serve template", http.StatusInternalServerError)
	}
}

func (h *Handler) ServeStaticFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}
	staticPath := r.URL.Path
	if strings.HasPrefix(staticPath, "/app/") {
		staticPath = strings.TrimPrefix(staticPath, "/app")
	}
	if staticPath == "/robots.txt" {
		serveRobotsTXT(w, r)
		return
	}
	if staticPath == "/sitemap.xml" {
		if err := serveSitemapXML(w, r); err != nil {
			http.Error(w, "Failed to build sitemap", http.StatusInternalServerError)
		}
		return
	}
	if err := web.ServeStatic(w, staticPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Failed to serve static file", http.StatusInternalServerError)
	}
}

func resolvePublicBaseURL(r *http.Request) string {
	if base, ok := configuredAppBaseURL(); ok {
		return base
	}
	if base := requestBaseURL(r); base != "" {
		return base
	}
	return "https://www.expenselog.com.ar"
}

func serveRobotsTXT(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(resolvePublicBaseURL(r), "/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	fmt.Fprintf(w, "User-agent: *\n")
	fmt.Fprintf(w, "Disallow: /app\n")
	fmt.Fprintf(w, "Disallow: /table\n")
	fmt.Fprintf(w, "Disallow: /settings\n")
	fmt.Fprintf(w, "Disallow: /auth/\n")
	fmt.Fprintf(w, "Disallow: /config\n")
	fmt.Fprintf(w, "Disallow: /categories\n")
	fmt.Fprintf(w, "Disallow: /expense\n")
	fmt.Fprintf(w, "Disallow: /expenses\n")
	fmt.Fprintf(w, "Disallow: /recurring-expense\n")
	fmt.Fprintf(w, "Disallow: /recurring-expenses\n")
	fmt.Fprintf(w, "Disallow: /import\n")
	fmt.Fprintf(w, "Disallow: /export\n")
	fmt.Fprintf(w, "Disallow: /api/\n\n")
	fmt.Fprintf(w, "Sitemap: %s/sitemap.xml\n", base)
}

func serveSitemapXML(w http.ResponseWriter, r *http.Request) error {
	type sitemapURL struct {
		Loc string `xml:"loc"`
	}
	type sitemapURLSet struct {
		XMLName xml.Name     `xml:"urlset"`
		Xmlns   string       `xml:"xmlns,attr"`
		URLs    []sitemapURL `xml:"url"`
	}
	base := strings.TrimRight(resolvePublicBaseURL(r), "/")
	payload := sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: []sitemapURL{
			{Loc: base + "/"},
		},
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "    ")
	if err := enc.Encode(payload); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, err := w.Write(buf.Bytes())
	return err
}
