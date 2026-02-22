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
	TransactionCount int
	Income           float64
	Expense          float64
	NetBalance       float64
	CardPending      float64
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

	query, err := h.parseMonthlyReportQuery(r, userID)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
		return
	}

	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve expenses"})
		return
	}

	filtered := filterExpensesForMonthlyReport(expenses, query)
	metrics := calculateMonthlyReportMetrics(filtered)

	buffer, err := buildMonthlyReportXLSX(filtered, query, metrics)
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

	query, err := h.parseMonthlyReportQuery(r, userID)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
		return
	}

	expenses, err := h.storage.GetAllExpenses(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve expenses"})
		return
	}

	filtered := filterExpensesForMonthlyReport(expenses, query)
	metrics := calculateMonthlyReportMetrics(filtered)

	buffer, err := buildMonthlyReportPDF(filtered, query, metrics)
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

func calculateMonthlyReportMetrics(expenses []storage.Expense) monthlyReportMetrics {
	var income float64
	var expense float64
	var rawCardTotals float64
	var ownerPayments float64

	for _, exp := range expenses {
		source := normalizeReportSource(exp.Source)
		amount := exp.Amount

		if source == "CA" {
			if amount > 0 {
				income += amount
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
	}

	rawDebt := math.Max(0, -rawCardTotals)
	cardPending := math.Max(0, rawDebt-ownerPayments)

	return monthlyReportMetrics{
		TransactionCount: len(expenses),
		Income:           income,
		Expense:          expense,
		NetBalance:       income - expense,
		CardPending:      cardPending,
	}
}

func buildMonthlyReportXLSX(expenses []storage.Expense, query monthlyReportQuery, metrics monthlyReportMetrics) (*bytes.Buffer, error) {
	file := excelize.NewFile()
	const movementsSheet = "Movimientos"
	const summarySheet = "Resumen"
	const categoriesSheet = "Categorias"

	file.SetSheetName("Sheet1", movementsSheet)
	_, _ = file.NewSheet(summarySheet)
	_, _ = file.NewSheet(categoriesSheet)

	headers := []string{"Fecha", "Nombre", "Tipo", "Categoria", "Monto", "Moneda", "Medio de pago", "Tarjeta", "Etiquetas", "Origen"}
	for idx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(idx+1, 1)
		_ = file.SetCellValue(movementsSheet, cell, header)
	}

	for idx, exp := range expenses {
		row := idx + 2
		values := []any{
			exp.Date.UTC().Format("2006-01-02"),
			exp.Name,
			formatFlowLabel(exp.Flow),
			exp.Category,
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
	}

	_ = file.SetColWidth(movementsSheet, "A", "A", 14)
	_ = file.SetColWidth(movementsSheet, "B", "D", 24)
	_ = file.SetColWidth(movementsSheet, "E", "G", 18)
	_ = file.SetColWidth(movementsSheet, "H", "J", 22)

	summaryRows := [][]any{
		{"Periodo", fmt.Sprintf("%04d-%02d", query.Year, query.Month)},
		{"Moneda", strings.ToUpper(query.Currency)},
		{"Movimientos", metrics.TransactionCount},
		{"Ingresos", metrics.Income},
		{"Egresos", metrics.Expense},
		{"Balance neto del periodo", metrics.NetBalance},
		{"Tarjeta por pagar (periodo)", metrics.CardPending},
	}
	for idx, values := range summaryRows {
		row := idx + 1
		cellA, _ := excelize.CoordinatesToCellName(1, row)
		cellB, _ := excelize.CoordinatesToCellName(2, row)
		_ = file.SetCellValue(summarySheet, cellA, values[0])
		_ = file.SetCellValue(summarySheet, cellB, values[1])
	}
	_ = file.SetColWidth(summarySheet, "A", "A", 30)
	_ = file.SetColWidth(summarySheet, "B", "B", 24)

	categoryTotals := map[string]struct {
		Count int
		Net   float64
	}{}
	for _, exp := range expenses {
		item := categoryTotals[exp.Category]
		item.Count++
		item.Net += exp.Amount
		categoryTotals[exp.Category] = item
	}
	type categoryRow struct {
		Name  string
		Count int
		Net   float64
	}
	categoryRows := make([]categoryRow, 0, len(categoryTotals))
	for name, values := range categoryTotals {
		categoryRows = append(categoryRows, categoryRow{Name: name, Count: values.Count, Net: values.Net})
	}
	sort.Slice(categoryRows, func(i, j int) bool {
		if categoryRows[i].Net == categoryRows[j].Net {
			return categoryRows[i].Name < categoryRows[j].Name
		}
		return categoryRows[i].Net > categoryRows[j].Net
	})

	_ = file.SetCellValue(categoriesSheet, "A1", "Categoria")
	_ = file.SetCellValue(categoriesSheet, "B1", "Cantidad")
	_ = file.SetCellValue(categoriesSheet, "C1", "Monto neto")
	for idx, row := range categoryRows {
		r := idx + 2
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("A%d", r), row.Name)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("B%d", r), row.Count)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("C%d", r), row.Net)
	}
	_ = file.SetColWidth(categoriesSheet, "A", "A", 24)
	_ = file.SetColWidth(categoriesSheet, "B", "C", 18)

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func buildMonthlyReportPDF(expenses []storage.Expense, query monthlyReportQuery, metrics monthlyReportMetrics) (*bytes.Buffer, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, "ExpenseLog - Reporte mensual", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Periodo: %04d-%02d | Moneda: %s", query.Year, query.Month, strings.ToUpper(query.Currency)), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Emitido: %s UTC", time.Now().UTC().Format("2006-01-02 15:04")), "", 1, "L", false, 0, "")

	pdf.Ln(2)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(0, 6, "Resumen", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 5, fmt.Sprintf("Movimientos: %d", metrics.TransactionCount), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Ingresos: %.2f", metrics.Income), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Egresos: %.2f", metrics.Expense), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Balance neto del periodo: %.2f", metrics.NetBalance), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Tarjeta por pagar (periodo): %.2f", metrics.CardPending), "", 1, "L", false, 0, "")

	pdf.Ln(2)
	pdf.SetFont("Arial", "B", 9)
	headers := []string{"Fecha", "Nombre", "Tipo", "Categoria", "Monto", "Medio"}
	widths := []float64{22, 48, 16, 36, 26, 28}
	for idx, header := range headers {
		pdf.CellFormat(widths[idx], 6, header, "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 8)
	for _, exp := range expenses {
		cells := []string{
			exp.Date.UTC().Format("2006-01-02"),
			trimForPDF(exp.Name, 28),
			trimForPDF(formatFlowLabel(exp.Flow), 12),
			trimForPDF(exp.Category, 22),
			fmt.Sprintf("%.2f", exp.Amount),
			trimForPDF(formatSourceLabel(exp.Source), 16),
		}
		for idx, value := range cells {
			align := "L"
			if idx == 4 {
				align = "R"
			}
			pdf.CellFormat(widths[idx], 6, value, "1", 0, align, false, 0, "")
		}
		pdf.Ln(-1)
	}

	buffer := bytes.NewBuffer(nil)
	if err := pdf.Output(buffer); err != nil {
		return nil, err
	}
	return buffer, nil
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
