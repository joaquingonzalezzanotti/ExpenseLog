package api

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
	"github.com/phpdave11/gofpdf"
	"github.com/xuri/excelize/v2"
)

const (
	reportCurrencyParam = "currency"
	reportYearParam     = "year"
	reportMonthParam    = "month"
)

type monthlyReportQuery struct {
	Year     int
	Month    int
	Currency string
	Start    time.Time
	End      time.Time
}

type monthlyReportMetrics struct {
	TransactionCount          int
	InitialBalance            float64
	Income                    float64
	Refund                    float64
	Expense                   float64
	NetBalance                float64
	CardPending               float64
	TotalOutflow              float64
	CashOutflow               float64
	CardOutflow               float64
	CashExpenseShare          float64
	CardExpenseShare          float64
	ActiveDays                int
	AvgDailyExpense           float64
	AvgExpenseTicket          float64
	MedianExpenseTicket       float64
	SavingsRate               float64
	CategoryConcentrationTop3 float64
	LargestExpenseName        string
	LargestExpenseAmount      float64
	LargestExpenseDate        time.Time
	LargestIncomeName         string
	LargestIncomeAmount       float64
	LargestIncomeDate         time.Time
}

type monthlyReportCategoryStat struct {
	Name         string
	Count        int
	ExpenseTotal float64
	IncomeTotal  float64
	Net          float64
	ExpenseShare float64
}

type monthlyReportInsight struct {
	Title  string
	Detail string
}

func (h *Handler) ExportMonthlyXLSX(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	isPremium, err := h.isPremiumUser(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate plan"})
		return
	}
	if !isPremium {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":   "La exportacion mensual es una funcion Premium.",
			"feature": "premium_required",
		})
		return
	}

	query, err := h.parseMonthlyReportQuery(r, userID)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
		return
	}

	expenses, err := h.storage.GetExpensesByPeriodAndCurrency(userID, query.Start, query.End, query.Currency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve expenses"})
		return
	}
	initialBalance, err := h.storage.GetCashBalanceBeforeDate(userID, query.Start, query.Currency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve initial balance"})
		return
	}

	categories := buildMonthlyReportCategoryStats(expenses)
	metrics := calculateMonthlyReportMetrics(expenses, categories, initialBalance)
	insights := buildMonthlyReportInsights(metrics, categories, query)

	buffer, err := buildMonthlyReportXLSX(expenses, query, metrics, categories, insights)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate XLSX report"})
		return
	}

	filename := buildMonthlyReportFilename("xlsx", query)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}

func (h *Handler) ExportMonthlyPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	isPremium, err := h.isPremiumUser(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate plan"})
		return
	}
	if !isPremium {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":   "La exportacion mensual es una funcion Premium.",
			"feature": "premium_required",
		})
		return
	}

	query, err := h.parseMonthlyReportQuery(r, userID)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
		return
	}

	expenses, err := h.storage.GetExpensesByPeriodAndCurrency(userID, query.Start, query.End, query.Currency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve expenses"})
		return
	}
	initialBalance, err := h.storage.GetCashBalanceBeforeDate(userID, query.Start, query.Currency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve initial balance"})
		return
	}

	categories := buildMonthlyReportCategoryStats(expenses)
	metrics := calculateMonthlyReportMetrics(expenses, categories, initialBalance)
	insights := buildMonthlyReportInsights(metrics, categories, query)

	buffer, err := buildMonthlyReportPDF(expenses, query, metrics, categories, insights)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate PDF report"})
		return
	}

	filename := buildMonthlyReportFilename("pdf", query)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}

func (h *Handler) parseMonthlyReportQuery(r *http.Request, userID string) (monthlyReportQuery, error) {
	now := time.Now().UTC()
	year := now.Year()
	month := int(now.Month())

	if y := strings.TrimSpace(r.URL.Query().Get(reportYearParam)); y != "" {
		parsedYear, err := strconv.Atoi(y)
		if err != nil {
			return monthlyReportQuery{}, fmt.Errorf("year parameter is invalid")
		}
		year = parsedYear
	}
	if m := strings.TrimSpace(r.URL.Query().Get(reportMonthParam)); m != "" {
		parsedMonth, err := strconv.Atoi(m)
		if err != nil {
			return monthlyReportQuery{}, fmt.Errorf("month parameter is invalid")
		}
		month = parsedMonth
	}

	if year < 2000 || year > 2100 {
		return monthlyReportQuery{}, fmt.Errorf("year must be between 2000 and 2100")
	}
	if month < 1 || month > 12 {
		return monthlyReportQuery{}, fmt.Errorf("month must be between 1 and 12")
	}

	currency := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(reportCurrencyParam)))
	if currency == "" {
		currentCurrency, err := h.storage.GetCurrency(userID)
		if err != nil {
			return monthlyReportQuery{}, fmt.Errorf("could not resolve currency")
		}
		currency = strings.ToLower(strings.TrimSpace(currentCurrency))
	}
	if !slices.Contains(storage.SupportedCurrencies, currency) {
		return monthlyReportQuery{}, fmt.Errorf("currency must be one of: ars, usd, eur")
	}

	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	return monthlyReportQuery{
		Year:     year,
		Month:    month,
		Currency: currency,
		Start:    start,
		End:      end,
	}, nil
}

func (h *Handler) isPremiumUser(userID string) (bool, error) {
	config, err := h.storage.GetConfig(userID)
	if err != nil {
		return false, err
	}
	return storage.NormalizePlanTier(config.PlanTier) == storage.PlanTierPremium, nil
}

func filterExpensesForMonthlyReport(expenses []storage.Expense, query monthlyReportQuery) []storage.Expense {
	filtered := make([]storage.Expense, 0)
	for _, exp := range expenses {
		expDate := exp.Date.UTC()
		if expDate.Before(query.Start) || !expDate.Before(query.End) {
			continue
		}
		currency := strings.ToLower(strings.TrimSpace(exp.Currency))
		if currency == "" {
			currency = query.Currency
		}
		if currency != query.Currency {
			continue
		}
		filtered = append(filtered, exp)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Date.Equal(filtered[j].Date) {
			return filtered[i].Name < filtered[j].Name
		}
		return filtered[i].Date.Before(filtered[j].Date)
	})

	return filtered
}

func buildMonthlyReportCategoryStats(expenses []storage.Expense) []monthlyReportCategoryStat {
	type bucket struct {
		count   int
		expense float64
		income  float64
		net     float64
	}
	byCategory := map[string]bucket{}

	for _, exp := range expenses {
		key := formatCategoryLabel(exp.Category)
		item := byCategory[key]
		item.count++
		if exp.Amount < 0 {
			item.expense += math.Abs(exp.Amount)
		} else if exp.Amount > 0 {
			item.income += exp.Amount
		}
		item.net += exp.Amount
		byCategory[key] = item
	}

	totalExpense := 0.0
	for _, item := range byCategory {
		totalExpense += item.expense
	}

	rows := make([]monthlyReportCategoryStat, 0, len(byCategory))
	for name, item := range byCategory {
		share := 0.0
		if totalExpense > 0 {
			share = (item.expense / totalExpense) * 100
		}
		rows = append(rows, monthlyReportCategoryStat{
			Name:         name,
			Count:        item.count,
			ExpenseTotal: item.expense,
			IncomeTotal:  item.income,
			Net:          item.net,
			ExpenseShare: share,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ExpenseTotal == rows[j].ExpenseTotal {
			if rows[i].Count == rows[j].Count {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].Count > rows[j].Count
		}
		return rows[i].ExpenseTotal > rows[j].ExpenseTotal
	})
	return rows
}

func calculateMonthlyReportMetrics(expenses []storage.Expense, categories []monthlyReportCategoryStat, initialBalance float64) monthlyReportMetrics {
	var income float64
	var refund float64
	var expense float64
	var rawCardTotals float64
	var ownerPayments float64
	var totalOutflow float64
	var cashOutflow float64
	var cardOutflow float64

	var largestExpenseAmount float64
	var largestExpenseName string
	var largestExpenseDate time.Time
	var largestIncomeAmount float64
	var largestIncomeName string
	var largestIncomeDate time.Time
	activeDays := map[string]struct{}{}
	expenseTickets := make([]float64, 0)
	sumExpenseTickets := 0.0

	for _, exp := range expenses {
		source := normalizeReportSource(exp.Source)
		amount := exp.Amount
		flow := normalizeReportFlow(exp.Flow, amount)
		activeDays[exp.Date.UTC().Format("2006-01-02")] = struct{}{}

		if source == "CA" {
			if amount > 0 {
				if flow == "refund" {
					refund += amount
				} else {
					income += amount
				}
			}
			if amount < 0 {
				expense += math.Abs(amount)
			}
		}

		if source == "TARJETA" {
			rawCardTotals += amount
		}

		if strings.EqualFold(strings.TrimSpace(exp.SystemOrigin), "card_payment_owner") && amount < 0 {
			ownerPayments += math.Abs(amount)
		}

		if amount < 0 {
			absAmount := math.Abs(amount)
			totalOutflow += absAmount
			expenseTickets = append(expenseTickets, absAmount)
			sumExpenseTickets += absAmount

			if source == "CA" {
				cashOutflow += absAmount
			}
			if source == "TARJETA" {
				cardOutflow += absAmount
			}
			if absAmount > largestExpenseAmount {
				largestExpenseAmount = absAmount
				largestExpenseName = strings.TrimSpace(exp.Name)
				largestExpenseDate = exp.Date.UTC()
			}
		}
		if amount > 0 && flow != "refund" && amount > largestIncomeAmount {
			largestIncomeAmount = amount
			largestIncomeName = strings.TrimSpace(exp.Name)
			largestIncomeDate = exp.Date.UTC()
		}
	}

	rawDebt := math.Max(0, -rawCardTotals)
	cardPending := math.Max(0, rawDebt-ownerPayments)

	cardShare := 0.0
	cashShare := 0.0
	if totalOutflow > 0 {
		cardShare = (cardOutflow / totalOutflow) * 100
		cashShare = (cashOutflow / totalOutflow) * 100
	}

	avgDailyExpense := 0.0
	if len(activeDays) > 0 {
		avgDailyExpense = totalOutflow / float64(len(activeDays))
	}

	avgExpenseTicket := 0.0
	medianExpenseTicket := 0.0
	if len(expenseTickets) > 0 {
		avgExpenseTicket = sumExpenseTickets / float64(len(expenseTickets))
		sortedTickets := append([]float64(nil), expenseTickets...)
		sort.Float64s(sortedTickets)
		middle := len(sortedTickets) / 2
		if len(sortedTickets)%2 == 0 {
			medianExpenseTicket = (sortedTickets[middle-1] + sortedTickets[middle]) / 2
		} else {
			medianExpenseTicket = sortedTickets[middle]
		}
	}

	availableCashIn := income + refund
	savingsRate := 0.0
	if availableCashIn > 0 {
		savingsRate = ((availableCashIn - expense) / availableCashIn) * 100
	}

	top3Share := 0.0
	for i, category := range categories {
		if i >= 3 {
			break
		}
		top3Share += category.ExpenseShare
	}

	return monthlyReportMetrics{
		TransactionCount:          len(expenses),
		InitialBalance:            initialBalance,
		Income:                    income,
		Refund:                    refund,
		Expense:                   expense,
		NetBalance:                initialBalance + income + refund - expense,
		CardPending:               cardPending,
		TotalOutflow:              totalOutflow,
		CashOutflow:               cashOutflow,
		CardOutflow:               cardOutflow,
		CashExpenseShare:          cashShare,
		CardExpenseShare:          cardShare,
		ActiveDays:                len(activeDays),
		AvgDailyExpense:           avgDailyExpense,
		AvgExpenseTicket:          avgExpenseTicket,
		MedianExpenseTicket:       medianExpenseTicket,
		SavingsRate:               savingsRate,
		CategoryConcentrationTop3: top3Share,
		LargestExpenseName:        largestExpenseName,
		LargestExpenseAmount:      largestExpenseAmount,
		LargestExpenseDate:        largestExpenseDate,
		LargestIncomeName:         largestIncomeName,
		LargestIncomeAmount:       largestIncomeAmount,
		LargestIncomeDate:         largestIncomeDate,
	}
}

func buildMonthlyReportInsights(metrics monthlyReportMetrics, categories []monthlyReportCategoryStat, query monthlyReportQuery) []monthlyReportInsight {
	insights := make([]monthlyReportInsight, 0, 6)
	if metrics.TransactionCount == 0 {
		return []monthlyReportInsight{
			{
				Title:  "Sin actividad",
				Detail: "No hay movimientos para el periodo seleccionado.",
			},
		}
	}

	switch {
	case metrics.SavingsRate < 0:
		insights = append(insights, monthlyReportInsight{
			Title:  "Resultado de caja",
			Detail: fmt.Sprintf("El mes cerro con deficit de caja (%.1f%%).", math.Abs(metrics.SavingsRate)),
		})
	case metrics.SavingsRate < 15:
		insights = append(insights, monthlyReportInsight{
			Title:  "Resultado de caja",
			Detail: fmt.Sprintf("El margen de ahorro fue bajo (%.1f%%).", metrics.SavingsRate),
		})
	default:
		insights = append(insights, monthlyReportInsight{
			Title:  "Resultado de caja",
			Detail: fmt.Sprintf("El ahorro de caja fue saludable (%.1f%%).", metrics.SavingsRate),
		})
	}

	if metrics.CategoryConcentrationTop3 >= 70 {
		insights = append(insights, monthlyReportInsight{
			Title:  "Concentracion de gasto",
			Detail: fmt.Sprintf("Las 3 categorias top explican %.1f%% del egreso.", metrics.CategoryConcentrationTop3),
		})
	} else {
		insights = append(insights, monthlyReportInsight{
			Title:  "Concentracion de gasto",
			Detail: fmt.Sprintf("Top 3 categorias representan %.1f%% del egreso.", metrics.CategoryConcentrationTop3),
		})
	}

	if metrics.CardExpenseShare > 55 {
		insights = append(insights, monthlyReportInsight{
			Title:  "Dependencia de tarjeta",
			Detail: fmt.Sprintf("El %.1f%% del egreso se hizo con tarjeta.", metrics.CardExpenseShare),
		})
	} else {
		insights = append(insights, monthlyReportInsight{
			Title:  "Mix de pago",
			Detail: fmt.Sprintf("Caja/debito %.1f%% vs tarjeta %.1f%% del egreso.", metrics.CashExpenseShare, metrics.CardExpenseShare),
		})
	}

	if metrics.AvgExpenseTicket > 0 && metrics.MedianExpenseTicket > 0 {
		ratio := metrics.AvgExpenseTicket / metrics.MedianExpenseTicket
		if ratio > 1.45 {
			insights = append(insights, monthlyReportInsight{
				Title:  "Dispersion de tickets",
				Detail: fmt.Sprintf("Promedio/mediana = %.2fx; hubo consumos puntuales altos.", ratio),
			})
		}
	}

	if len(categories) > 0 && categories[0].ExpenseTotal > 0 {
		insights = append(insights, monthlyReportInsight{
			Title: "Categoria dominante",
			Detail: fmt.Sprintf("%s concentra %.1f%% (%s).",
				categories[0].Name,
				categories[0].ExpenseShare,
				formatReportAmount(categories[0].ExpenseTotal, query.Currency),
			),
		})
	}

	if metrics.LargestExpenseAmount > 0 {
		name := strings.TrimSpace(metrics.LargestExpenseName)
		if name == "" {
			name = "Movimiento sin nombre"
		}
		insights = append(insights, monthlyReportInsight{
			Title: "Mayor egreso",
			Detail: fmt.Sprintf("%s por %s el %s.",
				name,
				formatReportAmount(metrics.LargestExpenseAmount, query.Currency),
				metrics.LargestExpenseDate.Format("2006-01-02"),
			),
		})
	}
	return insights
}

func buildMonthlyReportXLSX(expenses []storage.Expense, query monthlyReportQuery, metrics monthlyReportMetrics, categories []monthlyReportCategoryStat, insights []monthlyReportInsight) (*bytes.Buffer, error) {
	file := excelize.NewFile()
	const movementsSheet = "Movimientos"
	const summarySheet = "Resumen"
	const categoriesSheet = "Categorias"
	const analysisSheet = "Analisis"

	file.SetSheetName("Sheet1", movementsSheet)
	_, _ = file.NewSheet(summarySheet)
	_, _ = file.NewSheet(categoriesSheet)
	_, _ = file.NewSheet(analysisSheet)

	titleStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#0F172A", Size: 16},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#E0E7FF"}},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
	})
	subtitleStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#334155", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#EEF2FF"}},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
	})
	headerStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1D4ED8"}},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "#BFDBFE", Style: 1},
			{Type: "right", Color: "#BFDBFE", Style: 1},
			{Type: "top", Color: "#BFDBFE", Style: 1},
			{Type: "bottom", Color: "#BFDBFE", Style: 1},
		},
	})
	bodyStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#0F172A", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFFFFF"}},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "#E2E8F0", Style: 1},
			{Type: "right", Color: "#E2E8F0", Style: 1},
			{Type: "top", Color: "#E2E8F0", Style: 1},
			{Type: "bottom", Color: "#E2E8F0", Style: 1},
		},
	})
	oddRowStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#0F172A", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#F8FAFC"}},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "#E2E8F0", Style: 1},
			{Type: "right", Color: "#E2E8F0", Style: 1},
			{Type: "top", Color: "#E2E8F0", Style: 1},
			{Type: "bottom", Color: "#E2E8F0", Style: 1},
		},
	})
	amountStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#0F172A", Size: 10},
		Alignment: &excelize.Alignment{
			Horizontal: "right",
			Vertical:   "center",
		},
		NumFmt: 4,
		Border: []excelize.Border{
			{Type: "left", Color: "#E2E8F0", Style: 1},
			{Type: "right", Color: "#E2E8F0", Style: 1},
			{Type: "top", Color: "#E2E8F0", Style: 1},
			{Type: "bottom", Color: "#E2E8F0", Style: 1},
		},
	})
	amountOddStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#0F172A", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#F8FAFC"}},
		Alignment: &excelize.Alignment{
			Horizontal: "right",
			Vertical:   "center",
		},
		NumFmt: 4,
		Border: []excelize.Border{
			{Type: "left", Color: "#E2E8F0", Style: 1},
			{Type: "right", Color: "#E2E8F0", Style: 1},
			{Type: "top", Color: "#E2E8F0", Style: 1},
			{Type: "bottom", Color: "#E2E8F0", Style: 1},
		},
	})

	periodLabel := fmt.Sprintf("%s %d", monthlyReportMonthName(query.Month), query.Year)
	metadata := fmt.Sprintf("Moneda: %s | Emitido: %s UTC", strings.ToUpper(query.Currency), time.Now().UTC().Format("2006-01-02 15:04"))

	_ = file.SetCellValue(movementsSheet, "A1", "ExpenseLog | Reporte mensual profesional")
	_ = file.MergeCell(movementsSheet, "A1", "J1")
	_ = file.SetCellStyle(movementsSheet, "A1", "J1", titleStyle)
	_ = file.SetRowHeight(movementsSheet, 1, 28)

	_ = file.SetCellValue(movementsSheet, "A2", fmt.Sprintf("Periodo: %s", periodLabel))
	_ = file.SetCellValue(movementsSheet, "A3", metadata)
	_ = file.MergeCell(movementsSheet, "A2", "J2")
	_ = file.MergeCell(movementsSheet, "A3", "J3")
	_ = file.SetCellStyle(movementsSheet, "A2", "J3", subtitleStyle)

	headers := []string{"Fecha", "Nombre", "Tipo", "Categoria", "Monto", "Moneda", "Medio de pago", "Tarjeta", "Etiquetas", "Origen"}
	headerRow := 5
	for idx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(idx+1, headerRow)
		_ = file.SetCellValue(movementsSheet, cell, header)
	}
	headerRangeEnd, _ := excelize.CoordinatesToCellName(len(headers), headerRow)
	_ = file.SetCellStyle(movementsSheet, "A5", headerRangeEnd, headerStyle)

	dataStartRow := headerRow + 1
	for idx, exp := range expenses {
		row := idx + dataStartRow
		values := []any{
			exp.Date.UTC().Format("2006-01-02"),
			exp.Name,
			formatFlowLabelResolved(exp.Flow, exp.Amount),
			formatCategoryLabel(exp.Category),
			exp.Amount,
			strings.ToUpper(query.Currency),
			formatSourceLabel(exp.Source),
			exp.Card,
			strings.Join(exp.Tags, ", "),
			exp.SystemOrigin,
		}
		for colIdx, value := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, row)
			_ = file.SetCellValue(movementsSheet, cell, value)
		}

		rowStart, _ := excelize.CoordinatesToCellName(1, row)
		rowEnd, _ := excelize.CoordinatesToCellName(len(headers), row)
		amountCell, _ := excelize.CoordinatesToCellName(5, row)
		if idx%2 == 0 {
			_ = file.SetCellStyle(movementsSheet, rowStart, rowEnd, bodyStyle)
			_ = file.SetCellStyle(movementsSheet, amountCell, amountCell, amountStyle)
		} else {
			_ = file.SetCellStyle(movementsSheet, rowStart, rowEnd, oddRowStyle)
			_ = file.SetCellStyle(movementsSheet, amountCell, amountCell, amountOddStyle)
		}
	}

	lastDataRow := headerRow
	if len(expenses) > 0 {
		lastDataRow = dataStartRow + len(expenses) - 1
	}
	_ = file.AutoFilter(movementsSheet, fmt.Sprintf("A%d:J%d", headerRow, lastDataRow), nil)
	_ = file.SetPanes(movementsSheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      headerRow,
		TopLeftCell: fmt.Sprintf("A%d", dataStartRow),
		ActivePane:  "bottomLeft",
	})

	_ = file.SetColWidth(movementsSheet, "A", "A", 13)
	_ = file.SetColWidth(movementsSheet, "B", "B", 29)
	_ = file.SetColWidth(movementsSheet, "C", "C", 12)
	_ = file.SetColWidth(movementsSheet, "D", "D", 20)
	_ = file.SetColWidth(movementsSheet, "E", "E", 14)
	_ = file.SetColWidth(movementsSheet, "F", "F", 10)
	_ = file.SetColWidth(movementsSheet, "G", "G", 24)
	_ = file.SetColWidth(movementsSheet, "H", "H", 18)
	_ = file.SetColWidth(movementsSheet, "I", "I", 24)
	_ = file.SetColWidth(movementsSheet, "J", "J", 23)

	summaryTitleStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#0F172A", Size: 14},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#DBEAFE"}},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
	})
	summaryLabelStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#1E3A8A", Size: 10},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#EFF6FF"}},
		Border: []excelize.Border{
			{Type: "left", Color: "#BFDBFE", Style: 1},
			{Type: "right", Color: "#BFDBFE", Style: 1},
			{Type: "top", Color: "#BFDBFE", Style: 1},
			{Type: "bottom", Color: "#BFDBFE", Style: 1},
		},
	})
	summaryValueTextStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#0F172A", Size: 11},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFFFFF"}},
		Alignment: &excelize.Alignment{
			Horizontal: "right",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "#BFDBFE", Style: 1},
			{Type: "right", Color: "#BFDBFE", Style: 1},
			{Type: "top", Color: "#BFDBFE", Style: 1},
			{Type: "bottom", Color: "#BFDBFE", Style: 1},
		},
	})
	summaryValueMoneyStyle, _ := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#0F172A", Size: 11},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFFFFF"}},
		Alignment: &excelize.Alignment{
			Horizontal: "right",
			Vertical:   "center",
		},
		NumFmt: 4,
		Border: []excelize.Border{
			{Type: "left", Color: "#BFDBFE", Style: 1},
			{Type: "right", Color: "#BFDBFE", Style: 1},
			{Type: "top", Color: "#BFDBFE", Style: 1},
			{Type: "bottom", Color: "#BFDBFE", Style: 1},
		},
	})

	_ = file.SetCellValue(summarySheet, "A1", "Resumen ejecutivo mensual")
	_ = file.MergeCell(summarySheet, "A1", "D1")
	_ = file.SetCellStyle(summarySheet, "A1", "D1", summaryTitleStyle)
	_ = file.SetRowHeight(summarySheet, 1, 24)
	_ = file.SetCellValue(summarySheet, "A2", fmt.Sprintf("Periodo: %s", periodLabel))
	_ = file.SetCellValue(summarySheet, "C2", fmt.Sprintf("Moneda: %s", strings.ToUpper(query.Currency)))

	summaryRows := []struct {
		Label string
		Value any
		Kind  string
	}{
		{Label: "Saldo inicial", Value: metrics.InitialBalance, Kind: "money"},
		{Label: "Dias con actividad", Value: metrics.ActiveDays, Kind: "text"},
		{Label: "Ingresos", Value: metrics.Income, Kind: "money"},
		{Label: "Reintegros", Value: metrics.Refund, Kind: "money"},
		{Label: "Egresos de caja", Value: metrics.Expense, Kind: "money"},
		{Label: "Consumo con tarjeta", Value: metrics.CardOutflow, Kind: "money"},
		{Label: "Egresos totales", Value: metrics.TotalOutflow, Kind: "money"},
		{Label: "Balance neto de caja", Value: metrics.NetBalance, Kind: "money"},
		{Label: "Tarjeta por pagar (periodo)", Value: metrics.CardPending, Kind: "money"},
		{Label: "Movimientos", Value: metrics.TransactionCount, Kind: "text"},
		{Label: "Ticket promedio de egreso", Value: metrics.AvgExpenseTicket, Kind: "money"},
		{Label: "Ticket mediano de egreso", Value: metrics.MedianExpenseTicket, Kind: "money"},
		{Label: "Concentracion top 3", Value: fmt.Sprintf("%.1f%%", metrics.CategoryConcentrationTop3), Kind: "text"},
		{Label: "Tasa de ahorro", Value: fmt.Sprintf("%.1f%%", metrics.SavingsRate), Kind: "text"},
	}
	for idx, row := range summaryRows {
		r := idx + 4
		labelCell := fmt.Sprintf("A%d", r)
		valueCell := fmt.Sprintf("B%d", r)
		_ = file.SetCellValue(summarySheet, labelCell, row.Label)
		_ = file.SetCellValue(summarySheet, valueCell, row.Value)
		_ = file.SetCellStyle(summarySheet, labelCell, labelCell, summaryLabelStyle)
		if row.Kind == "money" {
			_ = file.SetCellStyle(summarySheet, valueCell, valueCell, summaryValueMoneyStyle)
		} else {
			_ = file.SetCellStyle(summarySheet, valueCell, valueCell, summaryValueTextStyle)
		}
	}
	_ = file.SetColWidth(summarySheet, "A", "A", 36)
	_ = file.SetColWidth(summarySheet, "B", "B", 23)
	_ = file.SetColWidth(summarySheet, "C", "D", 20)

	_ = file.SetCellValue(categoriesSheet, "A1", "Categorias destacadas")
	_ = file.MergeCell(categoriesSheet, "A1", "F1")
	_ = file.SetCellStyle(categoriesSheet, "A1", "F1", summaryTitleStyle)
	_ = file.SetCellValue(categoriesSheet, "A2", "La distribucion de egresos replica el criterio del grafico de torta del panel principal.")
	_ = file.MergeCell(categoriesSheet, "A2", "F2")
	_ = file.SetCellStyle(categoriesSheet, "A2", "F2", subtitleStyle)

	_ = file.SetCellValue(categoriesSheet, "A3", "Categoria")
	_ = file.SetCellValue(categoriesSheet, "B3", "Cantidad")
	_ = file.SetCellValue(categoriesSheet, "C3", "Egreso")
	_ = file.SetCellValue(categoriesSheet, "D3", "Ingreso")
	_ = file.SetCellValue(categoriesSheet, "E3", "Monto neto")
	_ = file.SetCellValue(categoriesSheet, "F3", "Participacion egresos")
	_ = file.SetCellStyle(categoriesSheet, "A3", "F3", headerStyle)

	lastExpenseShareRow := 0
	lastCategoryRow := 3
	for idx, row := range categories {
		r := idx + 4
		lastCategoryRow = r
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("A%d", r), row.Name)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("B%d", r), row.Count)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("C%d", r), row.ExpenseTotal)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("D%d", r), row.IncomeTotal)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("E%d", r), row.Net)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("F%d", r), fmt.Sprintf("%.1f%%", row.ExpenseShare))
		if row.ExpenseTotal > 0 {
			lastExpenseShareRow = r
		}
		startCell := fmt.Sprintf("A%d", r)
		endCell := fmt.Sprintf("F%d", r)
		if idx%2 == 0 {
			_ = file.SetCellStyle(categoriesSheet, startCell, endCell, bodyStyle)
			_ = file.SetCellStyle(categoriesSheet, fmt.Sprintf("C%d", r), fmt.Sprintf("E%d", r), amountStyle)
		} else {
			_ = file.SetCellStyle(categoriesSheet, startCell, endCell, oddRowStyle)
			_ = file.SetCellStyle(categoriesSheet, fmt.Sprintf("C%d", r), fmt.Sprintf("E%d", r), amountOddStyle)
		}
	}
	if len(categories) == 0 {
		_ = file.SetCellValue(categoriesSheet, "A4", "No hay categorias para el periodo seleccionado.")
		_ = file.MergeCell(categoriesSheet, "A4", "F4")
		_ = file.SetCellStyle(categoriesSheet, "A4", "F4", bodyStyle)
		lastCategoryRow = 4
	}

	_ = file.SetColWidth(categoriesSheet, "A", "A", 29)
	_ = file.SetColWidth(categoriesSheet, "B", "B", 11)
	_ = file.SetColWidth(categoriesSheet, "C", "E", 16)
	_ = file.SetColWidth(categoriesSheet, "F", "F", 20)

	if lastExpenseShareRow >= 4 {
		varyColors := true
		_ = file.AddChart(categoriesSheet, "H3", &excelize.Chart{
			Type: excelize.Doughnut,
			Series: []excelize.ChartSeries{
				{
					Name:       fmt.Sprintf("%s!$C$3", categoriesSheet),
					Categories: fmt.Sprintf("%s!$A$4:$A$%d", categoriesSheet, lastExpenseShareRow),
					Values:     fmt.Sprintf("%s!$C$4:$C$%d", categoriesSheet, lastExpenseShareRow),
				},
			},
			Title: []excelize.RichTextRun{
				{Text: "Distribucion de egresos por categoria"},
			},
			VaryColors: &varyColors,
			HoleSize:   62,
			Legend: excelize.ChartLegend{
				Position: "right",
			},
			PlotArea: excelize.ChartPlotArea{
				ShowPercent: true,
			},
			Dimension: excelize.ChartDimension{
				Width:  560,
				Height: 320,
			},
		})
	}

	_ = file.SetCellValue(analysisSheet, "A1", "Analisis avanzado del periodo")
	_ = file.MergeCell(analysisSheet, "A1", "E1")
	_ = file.SetCellStyle(analysisSheet, "A1", "E1", summaryTitleStyle)
	_ = file.SetCellValue(analysisSheet, "A2", fmt.Sprintf("Periodo: %s | Moneda: %s", periodLabel, strings.ToUpper(query.Currency)))
	_ = file.MergeCell(analysisSheet, "A2", "E2")
	_ = file.SetCellStyle(analysisSheet, "A2", "E2", subtitleStyle)
	_ = file.SetCellValue(analysisSheet, "A4", "Indicador")
	_ = file.SetCellValue(analysisSheet, "B4", "Valor")
	_ = file.SetCellStyle(analysisSheet, "A4", "B4", headerStyle)

	analysisRows := []struct {
		Label string
		Value any
		Kind  string
	}{
		{Label: "Egreso diario promedio", Value: metrics.AvgDailyExpense, Kind: "money"},
		{Label: "Ticket promedio", Value: metrics.AvgExpenseTicket, Kind: "money"},
		{Label: "Ticket mediano", Value: metrics.MedianExpenseTicket, Kind: "money"},
		{Label: "Share de gasto con tarjeta", Value: fmt.Sprintf("%.1f%%", metrics.CardExpenseShare), Kind: "text"},
		{Label: "Share de gasto de caja/debito", Value: fmt.Sprintf("%.1f%%", metrics.CashExpenseShare), Kind: "text"},
		{Label: "Concentracion top 3 categorias", Value: fmt.Sprintf("%.1f%%", metrics.CategoryConcentrationTop3), Kind: "text"},
		{Label: "Tarjeta por pagar", Value: metrics.CardPending, Kind: "money"},
	}
	rowCursor := 5
	for _, row := range analysisRows {
		labelCell := fmt.Sprintf("A%d", rowCursor)
		valueCell := fmt.Sprintf("B%d", rowCursor)
		_ = file.SetCellValue(analysisSheet, labelCell, row.Label)
		_ = file.SetCellValue(analysisSheet, valueCell, row.Value)
		_ = file.SetCellStyle(analysisSheet, labelCell, labelCell, summaryLabelStyle)
		if row.Kind == "money" {
			_ = file.SetCellStyle(analysisSheet, valueCell, valueCell, summaryValueMoneyStyle)
		} else {
			_ = file.SetCellStyle(analysisSheet, valueCell, valueCell, summaryValueTextStyle)
		}
		rowCursor++
	}

	if metrics.LargestExpenseAmount > 0 {
		_ = file.SetCellValue(analysisSheet, "D4", "Mayor egreso")
		_ = file.SetCellStyle(analysisSheet, "D4", "E4", headerStyle)
		_ = file.SetCellValue(analysisSheet, "D5", strings.TrimSpace(metrics.LargestExpenseName))
		_ = file.SetCellValue(analysisSheet, "E5", formatReportAmount(metrics.LargestExpenseAmount, query.Currency))
		_ = file.SetCellValue(analysisSheet, "D6", "Fecha")
		_ = file.SetCellValue(analysisSheet, "E6", metrics.LargestExpenseDate.Format("2006-01-02"))
		_ = file.SetCellStyle(analysisSheet, "D5", "E6", bodyStyle)
	}
	if metrics.LargestIncomeAmount > 0 {
		_ = file.SetCellValue(analysisSheet, "D8", "Mayor ingreso")
		_ = file.SetCellStyle(analysisSheet, "D8", "E8", headerStyle)
		_ = file.SetCellValue(analysisSheet, "D9", strings.TrimSpace(metrics.LargestIncomeName))
		_ = file.SetCellValue(analysisSheet, "E9", formatReportAmount(metrics.LargestIncomeAmount, query.Currency))
		_ = file.SetCellValue(analysisSheet, "D10", "Fecha")
		_ = file.SetCellValue(analysisSheet, "E10", metrics.LargestIncomeDate.Format("2006-01-02"))
		_ = file.SetCellStyle(analysisSheet, "D9", "E10", bodyStyle)
	}

	insightStart := rowCursor + 1
	_ = file.SetCellValue(analysisSheet, fmt.Sprintf("A%d", insightStart), "Insights inteligentes")
	_ = file.MergeCell(analysisSheet, fmt.Sprintf("A%d", insightStart), fmt.Sprintf("E%d", insightStart))
	_ = file.SetCellStyle(analysisSheet, fmt.Sprintf("A%d", insightStart), fmt.Sprintf("E%d", insightStart), summaryTitleStyle)
	if len(insights) == 0 {
		_ = file.SetCellValue(analysisSheet, fmt.Sprintf("A%d", insightStart+1), "No hubo suficientes datos para generar insights.")
		_ = file.MergeCell(analysisSheet, fmt.Sprintf("A%d", insightStart+1), fmt.Sprintf("E%d", insightStart+1))
		_ = file.SetCellStyle(analysisSheet, fmt.Sprintf("A%d", insightStart+1), fmt.Sprintf("E%d", insightStart+1), bodyStyle)
	} else {
		for idx, insight := range insights {
			r := insightStart + 1 + idx
			_ = file.SetCellValue(analysisSheet, fmt.Sprintf("A%d", r), fmt.Sprintf("%d.", idx+1))
			_ = file.SetCellValue(analysisSheet, fmt.Sprintf("B%d", r), insight.Title)
			_ = file.SetCellValue(analysisSheet, fmt.Sprintf("C%d", r), insight.Detail)
			_ = file.MergeCell(analysisSheet, fmt.Sprintf("C%d", r), fmt.Sprintf("E%d", r))
			_ = file.SetCellStyle(analysisSheet, fmt.Sprintf("A%d", r), fmt.Sprintf("E%d", r), bodyStyle)
		}
	}
	_ = file.SetColWidth(analysisSheet, "A", "A", 6)
	_ = file.SetColWidth(analysisSheet, "B", "B", 30)
	_ = file.SetColWidth(analysisSheet, "C", "E", 36)

	if len(categories) > 0 && lastCategoryRow >= 4 {
		_ = file.AutoFilter(categoriesSheet, fmt.Sprintf("A3:F%d", lastCategoryRow), nil)
	}

	file.SetActiveSheet(1)

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func buildMonthlyReportPDF(
	expenses []storage.Expense,
	query monthlyReportQuery,
	metrics monthlyReportMetrics,
	categories []monthlyReportCategoryStat,
	insights []monthlyReportInsight,
) (*bytes.Buffer, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-9)
		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(0, 5, fmt.Sprintf("ExpenseLog | Pagina %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	drawPDFSummaryPage(pdf, query, metrics, categories)

	// Page 2+ : full movement table (continues automatically if needed).
	pdf.AddPage()
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, "Detalle de movimientos", "", 1, "L", false, 0, "")

	headers := []string{"Fecha", "Nombre", "Tipo", "Categoria", "Monto", "Medio de pago"}
	widths := []float64{21, 52, 18, 30, 34, 35}
	drawPDFReportTableHeader(pdf, headers, widths)

	if len(expenses) == 0 {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(0, 8, "No hay movimientos para el periodo seleccionado.", "1", 1, "L", false, 0, "")
	}

	pdf.SetFont("Arial", "", 8.5)
	for idx, exp := range expenses {
		if pdf.GetY()+6 > 285 {
			pdf.AddPage()
			pdf.SetTextColor(15, 23, 42)
			pdf.SetFont("Arial", "B", 11)
			pdf.CellFormat(0, 7, "Detalle de movimientos (continuacion)", "", 1, "L", false, 0, "")
			drawPDFReportTableHeader(pdf, headers, widths)
			pdf.SetFont("Arial", "", 8.5)
		}
		if idx%2 == 0 {
			pdf.SetFillColor(255, 255, 255)
		} else {
			pdf.SetFillColor(248, 250, 252)
		}
		pdf.SetTextColor(15, 23, 42)
		row := []string{
			exp.Date.UTC().Format("2006-01-02"),
			trimForPDF(exp.Name, 34),
			trimForPDF(formatFlowLabelResolved(exp.Flow, exp.Amount), 14),
			trimForPDF(formatCategoryLabel(exp.Category), 18),
			formatReportAmount(exp.Amount, query.Currency),
			trimForPDF(formatSourceLabel(exp.Source), 20),
		}
		for col, value := range row {
			align := "L"
			if col == 4 {
				align = "R"
			}
			pdf.CellFormat(widths[col], 6, value, "1", 0, align, true, 0, "")
		}
		pdf.Ln(-1)
	}

	// Last page : additional chart + economic analysis.
	pdf.AddPage()
	drawPDFAnalysisPage(pdf, query, metrics, categories, insights)

	buffer := bytes.NewBuffer(nil)
	if err := pdf.Output(buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

func drawPDFSummaryPage(pdf *gofpdf.Fpdf, query monthlyReportQuery, metrics monthlyReportMetrics, categories []monthlyReportCategoryStat) {
	pageLeft := 10.0
	pageWidth := 190.0
	topY := 10.0

	pdf.SetFillColor(29, 78, 216)
	pdf.Rect(pageLeft, topY, pageWidth, 16, "F")
	pdf.SetXY(pageLeft+3, topY+3.5)
	pdf.SetFont("Arial", "B", 15)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(120, 6, "ExpenseLog | Reporte mensual", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(65, 6, fmt.Sprintf("%s %d", monthlyReportMonthName(query.Month), query.Year), "", 1, "R", false, 0, "")

	pdf.SetTextColor(51, 65, 85)
	pdf.SetXY(pageLeft+3, 31)
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 5, fmt.Sprintf("Moneda: %s | Emitido: %s UTC", strings.ToUpper(query.Currency), time.Now().UTC().Format("2006-01-02 15:04")), "", 1, "L", false, 0, "")

	cards := []struct {
		Title      string
		Value      string
		FillColor  [3]int
		LineColor  [3]int
		TitleColor [3]int
	}{
		{
			Title:      "Saldo inicial",
			Value:      formatReportAmount(metrics.InitialBalance, query.Currency),
			FillColor:  [3]int{239, 246, 255},
			LineColor:  [3]int{147, 197, 253},
			TitleColor: [3]int{30, 64, 175},
		},
		{
			Title:      "Ingresos",
			Value:      formatReportAmount(metrics.Income+metrics.Refund, query.Currency),
			FillColor:  [3]int{236, 253, 245},
			LineColor:  [3]int{110, 231, 183},
			TitleColor: [3]int{6, 95, 70},
		},
		{
			Title:      "Egresos",
			Value:      formatReportAmount(metrics.Expense, query.Currency),
			FillColor:  [3]int{254, 242, 242},
			LineColor:  [3]int{252, 165, 165},
			TitleColor: [3]int{153, 27, 27},
		},
		{
			Title:      "Balance neto",
			Value:      formatReportAmount(metrics.NetBalance, query.Currency),
			FillColor:  [3]int{238, 242, 255},
			LineColor:  [3]int{165, 180, 252},
			TitleColor: [3]int{55, 48, 163},
		},
		{
			Title:      "Gasto tarjeta",
			Value:      formatReportAmount(metrics.CardOutflow, query.Currency),
			FillColor:  [3]int{255, 251, 235},
			LineColor:  [3]int{253, 186, 116},
			TitleColor: [3]int{146, 64, 14},
		},
		{
			Title:      "Movimientos",
			Value:      fmt.Sprintf("%d", metrics.TransactionCount),
			FillColor:  [3]int{243, 244, 246},
			LineColor:  [3]int{209, 213, 219},
			TitleColor: [3]int{55, 65, 81},
		},
	}

	cardW := 91.0
	cardH := 16.0
	cardGap := 8.0
	startCardY := 38.0
	for idx, card := range cards {
		col := idx % 2
		row := idx / 2
		x := pageLeft + float64(col)*(cardW+cardGap)
		y := startCardY + float64(row)*(cardH+5)

		pdf.SetDrawColor(card.LineColor[0], card.LineColor[1], card.LineColor[2])
		pdf.SetFillColor(card.FillColor[0], card.FillColor[1], card.FillColor[2])
		pdf.RoundedRect(x, y, cardW, cardH, 2, "1234", "DF")
		pdf.SetXY(x+3, y+3)
		pdf.SetTextColor(card.TitleColor[0], card.TitleColor[1], card.TitleColor[2])
		pdf.SetFont("Arial", "B", 8.8)
		pdf.CellFormat(cardW-6, 4, card.Title, "", 2, "L", false, 0, "")
		pdf.SetX(x + 3)
		pdf.SetTextColor(15, 23, 42)
		pdf.SetFont("Arial", "B", 10.8)
		pdf.CellFormat(cardW-6, 5, card.Value, "", 0, "L", false, 0, "")
	}

	topCategories := topCategoryStatsByExpense(categories, 6)
	pieSectionY := startCardY + 3*(cardH+5) + 8
	pdf.SetY(pieSectionY)
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10.5)
	pdf.CellFormat(0, 6, "Distribucion por categoria (egresos)", "", 1, "L", false, 0, "")

	if len(topCategories) == 0 {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(0, 6, "No hay egresos para el periodo.", "1", 1, "L", false, 0, "")
		return
	}

	drawPDFPieChart(pdf, 40, pieSectionY+28, 22, topCategories)
	drawPDFPieLegend(pdf, 67, pieSectionY+7, topCategories, query.Currency)
	drawPDFCategorySectionTable(pdf, pieSectionY+55, topCategories, query.Currency)
}

func drawPDFCategorySectionTable(pdf *gofpdf.Fpdf, y float64, categories []monthlyReportCategoryStat, currency string) {
	pdf.SetY(y)
	headers := []string{"Categoria", "Egreso", "Share"}
	widths := []float64{110, 45, 35}
	pdf.SetFont("Arial", "B", 9)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFillColor(30, 64, 175)
	for idx, header := range headers {
		align := "L"
		if idx > 0 {
			align = "R"
		}
		pdf.CellFormat(widths[idx], 6, header, "1", 0, align, true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 8.7)
	for idx, row := range categories {
		if idx%2 == 0 {
			pdf.SetFillColor(255, 255, 255)
		} else {
			pdf.SetFillColor(248, 250, 252)
		}
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(widths[0], 6, trimForPDF(row.Name, 44), "1", 0, "L", true, 0, "")
		pdf.CellFormat(widths[1], 6, formatReportAmount(row.ExpenseTotal, currency), "1", 0, "R", true, 0, "")
		pdf.CellFormat(widths[2], 6, fmt.Sprintf("%.1f%%", row.ExpenseShare), "1", 1, "R", true, 0, "")
	}
}

func drawPDFPieLegend(pdf *gofpdf.Fpdf, x, y float64, categories []monthlyReportCategoryStat, currency string) {
	palette := monthlyReportPalette()
	pdf.SetFont("Arial", "", 8.3)
	for i, row := range categories {
		color := palette[i%len(palette)]
		currentY := y + float64(i)*6.2
		pdf.SetFillColor(color[0], color[1], color[2])
		pdf.Rect(x, currentY+1.6, 2.8, 2.8, "F")
		pdf.SetTextColor(51, 65, 85)
		label := fmt.Sprintf("%s (%.1f%%)", trimForPDF(row.Name, 22), row.ExpenseShare)
		pdf.SetXY(x+4.2, currentY)
		pdf.CellFormat(52, 5, label, "", 0, "L", false, 0, "")
		pdf.SetXY(x+56, currentY)
		pdf.CellFormat(26, 5, formatReportAmount(row.ExpenseTotal, currency), "", 1, "R", false, 0, "")
	}
}

func drawPDFPieChart(pdf *gofpdf.Fpdf, cx, cy, radius float64, categories []monthlyReportCategoryStat) {
	total := 0.0
	for _, row := range categories {
		total += row.ExpenseTotal
	}
	if total <= 0 {
		pdf.SetDrawColor(203, 213, 225)
		pdf.SetFillColor(248, 250, 252)
		pdf.Circle(cx, cy, radius, "DF")
		return
	}

	palette := monthlyReportPalette()
	startDeg := -90.0
	for i, row := range categories {
		if row.ExpenseTotal <= 0 {
			continue
		}
		sweep := (row.ExpenseTotal / total) * 360
		endDeg := startDeg + sweep
		points := []gofpdf.PointType{{X: cx, Y: cy}}
		steps := int(math.Max(8, math.Ceil(sweep/4)))
		for s := 0; s <= steps; s++ {
			deg := startDeg + (sweep*float64(s))/float64(steps)
			rad := deg * math.Pi / 180
			points = append(points, gofpdf.PointType{
				X: cx + radius*math.Cos(rad),
				Y: cy + radius*math.Sin(rad),
			})
		}
		points = append(points, gofpdf.PointType{
			X: cx + radius*math.Cos(endDeg*math.Pi/180),
			Y: cy + radius*math.Sin(endDeg*math.Pi/180),
		})
		color := palette[i%len(palette)]
		pdf.SetFillColor(color[0], color[1], color[2])
		pdf.SetDrawColor(255, 255, 255)
		pdf.Polygon(points, "FD")
		startDeg = endDeg
	}
}

func drawPDFAnalysisPage(pdf *gofpdf.Fpdf, query monthlyReportQuery, metrics monthlyReportMetrics, categories []monthlyReportCategoryStat, insights []monthlyReportInsight) {
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, "Analisis economico del mes", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(71, 85, 105)
	pdf.CellFormat(0, 5, fmt.Sprintf("Periodo: %s %d | Moneda: %s", monthlyReportMonthName(query.Month), query.Year, strings.ToUpper(query.Currency)), "", 1, "L", false, 0, "")

	drawPDFWaterfallChart(pdf, 12, 28, 186, 74, metrics, query.Currency)
	drawPDFTopCategoriesBarChart(pdf, 12, 112, 186, 62, categories, query.Currency)
	drawPDFEconomicSummary(pdf, 12, 182, 186, metrics, insights, query.Currency)
}

func drawPDFWaterfallChart(pdf *gofpdf.Fpdf, x, y, w, h float64, metrics monthlyReportMetrics, currency string) {
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetXY(x, y-6)
	pdf.CellFormat(w, 5, "Waterfall de resultado de caja", "", 0, "L", false, 0, "")

	incomes := metrics.Income + metrics.Refund
	values := []float64{
		metrics.InitialBalance,
		metrics.InitialBalance + incomes,
		metrics.NetBalance,
	}
	minV := math.Min(0, math.Min(values[0], math.Min(values[1], values[2])))
	maxV := math.Max(0, math.Max(values[0], math.Max(values[1], values[2])))
	if math.Abs(maxV-minV) < 0.01 {
		maxV = minV + 1
	}
	padding := (maxV - minV) * 0.1
	minV -= padding
	maxV += padding

	toY := func(v float64) float64 {
		return y + h - ((v-minV)/(maxV-minV))*h
	}
	zeroY := toY(0)
	pdf.SetDrawColor(203, 213, 225)
	pdf.Line(x, zeroY, x+w, zeroY)
	pdf.Rect(x, y, w, h, "D")

	barW := 28.0
	gap := (w - 4*barW) / 5
	xs := []float64{
		x + gap,
		x + gap*2 + barW,
		x + gap*3 + barW*2,
		x + gap*4 + barW*3,
	}

	bars := []struct {
		label string
		from  float64
		to    float64
		color [3]int
	}{
		{"Saldo inicial", 0, metrics.InitialBalance, [3]int{59, 130, 246}},
		{"+ Ingresos", metrics.InitialBalance, metrics.InitialBalance + incomes, [3]int{16, 185, 129}},
		{"- Egresos", metrics.InitialBalance + incomes, metrics.NetBalance, [3]int{239, 68, 68}},
		{"Cierre", 0, metrics.NetBalance, [3]int{99, 102, 241}},
	}

	pdf.SetFont("Arial", "", 7.8)
	for i, bar := range bars {
		y1 := toY(bar.from)
		y2 := toY(bar.to)
		top := math.Min(y1, y2)
		height := math.Abs(y2 - y1)
		if height < 1 {
			height = 1
		}
		pdf.SetFillColor(bar.color[0], bar.color[1], bar.color[2])
		pdf.SetDrawColor(255, 255, 255)
		pdf.Rect(xs[i], top, barW, height, "FD")
		pdf.SetTextColor(51, 65, 85)
		pdf.SetXY(xs[i], y+h+1.5)
		pdf.CellFormat(barW, 4, trimForPDF(bar.label, 14), "", 2, "C", false, 0, "")
		pdf.SetX(xs[i])
		amount := bar.to
		if i == 1 {
			amount = incomes
		}
		if i == 2 {
			amount = -metrics.Expense
		}
		pdf.CellFormat(barW, 4, trimForPDF(formatReportAmount(amount, currency), 14), "", 0, "C", false, 0, "")
	}
}

func drawPDFTopCategoriesBarChart(pdf *gofpdf.Fpdf, x, y, w, h float64, categories []monthlyReportCategoryStat, currency string) {
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetXY(x, y-6)
	pdf.CellFormat(w, 5, "Top 5 categorias de egreso", "", 0, "L", false, 0, "")
	pdf.Rect(x, y, w, h, "D")

	top := topCategoryStatsByExpense(categories, 5)
	if len(top) == 0 {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.SetXY(x+3, y+h/2-2)
		pdf.CellFormat(w-6, 4, "Sin egresos para mostrar.", "", 0, "L", false, 0, "")
		return
	}

	maxExpense := top[0].ExpenseTotal
	leftLabelW := 56.0
	barAreaW := 88.0
	valueW := 34.0
	rowH := h / float64(len(top))
	palette := monthlyReportPalette()
	pdf.SetFont("Arial", "", 8.4)
	for i, row := range top {
		rowY := y + float64(i)*rowH
		pdf.SetTextColor(51, 65, 85)
		pdf.SetXY(x+2, rowY+1.4)
		pdf.CellFormat(leftLabelW-2, 4.5, trimForPDF(row.Name, 20), "", 0, "L", false, 0, "")
		ratio := 0.0
		if maxExpense > 0 {
			ratio = row.ExpenseTotal / maxExpense
		}
		barW := math.Max(1, barAreaW*ratio)
		color := palette[i%len(palette)]
		pdf.SetFillColor(color[0], color[1], color[2])
		pdf.Rect(x+leftLabelW, rowY+1.5, barW, rowH-3, "F")
		pdf.SetXY(x+leftLabelW+barAreaW+2, rowY+1.2)
		pdf.CellFormat(valueW-2, 4.5, formatReportAmount(row.ExpenseTotal, currency), "", 0, "R", false, 0, "")
	}
}

func drawPDFEconomicSummary(pdf *gofpdf.Fpdf, x, y, w float64, metrics monthlyReportMetrics, insights []monthlyReportInsight, currency string) {
	pdf.SetXY(x, y)
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(w, 5, "Conclusion economica", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(51, 65, 85)
	baseLines := []string{
		fmt.Sprintf("- Balance neto de caja: %s", formatReportAmount(metrics.NetBalance, currency)),
		fmt.Sprintf("- Tasa de ahorro: %.1f%% | Dependencia tarjeta: %.1f%%", metrics.SavingsRate, metrics.CardExpenseShare),
		fmt.Sprintf("- Ticket promedio: %s | Mediana: %s", formatReportAmount(metrics.AvgExpenseTicket, currency), formatReportAmount(metrics.MedianExpenseTicket, currency)),
		fmt.Sprintf("- Concentracion top 3 categorias: %.1f%%", metrics.CategoryConcentrationTop3),
	}
	for _, line := range baseLines {
		pdf.MultiCell(w, 4.8, line, "", "L", false)
	}

	if len(insights) > 0 {
		pdf.Ln(1)
		pdf.SetFont("Arial", "B", 9)
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(w, 4.8, "Hallazgos clave", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 8.8)
		pdf.SetTextColor(51, 65, 85)
		limit := 3
		if len(insights) < limit {
			limit = len(insights)
		}
		for i := 0; i < limit; i++ {
			pdf.MultiCell(w, 4.8, fmt.Sprintf("- %s: %s", insights[i].Title, insights[i].Detail), "", "L", false)
		}
	}
}

func monthlyReportPalette() [][3]int {
	return [][3]int{
		{59, 130, 246},
		{16, 185, 129},
		{244, 114, 182},
		{245, 158, 11},
		{139, 92, 246},
		{34, 197, 94},
		{14, 165, 233},
	}
}

func drawPDFCategorySection(pdf *gofpdf.Fpdf, categories []monthlyReportCategoryStat, currency string) {
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10.5)
	pdf.CellFormat(0, 6, "Distribucion por categoria (egresos)", "", 1, "L", false, 0, "")

	top := topCategoryStatsByExpense(categories, 6)
	if len(top) == 0 {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(0, 6, "No hay egresos para el periodo.", "1", 1, "L", false, 0, "")
		return
	}

	headers := []string{"Categoria", "Egreso", "Share"}
	widths := []float64{110, 45, 35}
	pdf.SetFont("Arial", "B", 9)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFillColor(30, 64, 175)
	for idx, header := range headers {
		align := "L"
		if idx > 0 {
			align = "R"
		}
		pdf.CellFormat(widths[idx], 6, header, "1", 0, align, true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 8.7)
	for idx, row := range top {
		if idx%2 == 0 {
			pdf.SetFillColor(255, 255, 255)
		} else {
			pdf.SetFillColor(248, 250, 252)
		}
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(widths[0], 6, trimForPDF(row.Name, 44), "1", 0, "L", true, 0, "")
		pdf.CellFormat(widths[1], 6, formatReportAmount(row.ExpenseTotal, currency), "1", 0, "R", true, 0, "")
		pdf.CellFormat(widths[2], 6, fmt.Sprintf("%.1f%%", row.ExpenseShare), "1", 1, "R", true, 0, "")
	}
}

func drawPDFInsightsSection(pdf *gofpdf.Fpdf, insights []monthlyReportInsight) {
	pdf.Ln(3)
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "B", 10.5)
	pdf.CellFormat(0, 6, "Analisis inteligente", "", 1, "L", false, 0, "")

	if len(insights) == 0 {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(0, 5, "- No hubo suficientes datos para emitir conclusiones.", "", 1, "L", false, 0, "")
		return
	}

	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(51, 65, 85)
	limit := 6
	if len(insights) < limit {
		limit = len(insights)
	}
	for i := 0; i < limit; i++ {
		line := fmt.Sprintf("- %s: %s", insights[i].Title, insights[i].Detail)
		pdf.MultiCell(0, 5, line, "", "L", false)
	}
}

func topCategoryStatsByExpense(rows []monthlyReportCategoryStat, limit int) []monthlyReportCategoryStat {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	result := make([]monthlyReportCategoryStat, 0, limit)
	for _, row := range rows {
		if row.ExpenseTotal <= 0 {
			continue
		}
		result = append(result, row)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func monthlyReportMonthName(month int) string {
	names := []string{
		"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
		"Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
	}
	if month < 1 || month > len(names) {
		return fmt.Sprintf("Mes %d", month)
	}
	return names[month-1]
}

func drawPDFReportTableHeader(pdf *gofpdf.Fpdf, headers []string, widths []float64) {
	pdf.SetFont("Arial", "B", 8.8)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFillColor(30, 64, 175)
	for idx, header := range headers {
		align := "L"
		if idx == 4 {
			align = "R"
		}
		pdf.CellFormat(widths[idx], 6, header, "1", 0, align, true, 0, "")
	}
	pdf.Ln(-1)
}

func formatReportAmount(amount float64, currency string) string {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" {
		code = "ARS"
	}
	sign := ""
	if amount < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s%s %s", sign, code, formatReportNumber(math.Abs(amount)))
}

func formatReportNumber(value float64) string {
	base := fmt.Sprintf("%.2f", value)
	parts := strings.SplitN(base, ".", 2)
	intPart := parts[0]
	decPart := "00"
	if len(parts) == 2 {
		decPart = parts[1]
	}
	var groups []string
	for len(intPart) > 3 {
		groups = append([]string{intPart[len(intPart)-3:]}, groups...)
		intPart = intPart[:len(intPart)-3]
	}
	if intPart != "" {
		groups = append([]string{intPart}, groups...)
	}
	return strings.Join(groups, ".") + "," + decPart
}

func buildMonthlyReportFilename(ext string, query monthlyReportQuery) string {
	return fmt.Sprintf("expenselog-reporte-%04d-%02d-%s.%s", query.Year, query.Month, query.Currency, ext)
}

func normalizeReportSource(source string) string {
	normalized := strings.ToUpper(strings.TrimSpace(source))
	if normalized == "" {
		return "CA"
	}
	return normalized
}

func formatSourceLabel(source string) string {
	switch normalizeReportSource(source) {
	case "CA":
		return "Transferencia / Debito"
	case "TARJETA":
		return "Tarjeta credito"
	case "EFECTIVO":
		return "Efectivo"
	default:
		return "Otro"
	}
}

func formatFlowLabel(flow string) string {
	switch strings.ToLower(strings.TrimSpace(flow)) {
	case "income":
		return "Ingreso"
	case "refund":
		return "Reintegro"
	default:
		return "Gasto"
	}
}

func formatFlowLabelResolved(flow string, amount float64) string {
	return formatFlowLabel(normalizeReportFlow(flow, amount))
}

func formatCategoryLabel(category string) string {
	trimmed := strings.TrimSpace(category)
	if trimmed == "" {
		return "Sin categoria"
	}
	if strings.EqualFold(trimmed, "_conciliacion") {
		return "Conciliacion"
	}
	return trimmed
}

func normalizeReportFlow(flow string, amount float64) string {
	normalized := strings.ToLower(strings.TrimSpace(flow))
	if normalized != "" {
		return normalized
	}
	if amount >= 0 {
		return "income"
	}
	return "expense"
}

func trimForPDF(value string, max int) string {
	trimmed := strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= max {
		return trimmed
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "."
}
