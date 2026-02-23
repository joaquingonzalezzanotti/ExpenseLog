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
		Label   string
		Value   any
		IsMoney bool
	}{
		{Label: "Movimientos", Value: metrics.TransactionCount, IsMoney: false},
		{Label: "Ingresos", Value: metrics.Income, IsMoney: true},
		{Label: "Egresos", Value: metrics.Expense, IsMoney: true},
		{Label: "Balance neto del periodo", Value: metrics.NetBalance, IsMoney: true},
		{Label: "Tarjeta por pagar (periodo)", Value: metrics.CardPending, IsMoney: true},
	}
	for idx, row := range summaryRows {
		r := idx + 4
		labelCell := fmt.Sprintf("A%d", r)
		valueCell := fmt.Sprintf("B%d", r)
		_ = file.SetCellValue(summarySheet, labelCell, row.Label)
		_ = file.SetCellValue(summarySheet, valueCell, row.Value)
		_ = file.SetCellStyle(summarySheet, labelCell, labelCell, summaryLabelStyle)
		if row.IsMoney {
			_ = file.SetCellStyle(summarySheet, valueCell, valueCell, summaryValueMoneyStyle)
		} else {
			_ = file.SetCellStyle(summarySheet, valueCell, valueCell, summaryValueTextStyle)
		}
	}
	_ = file.SetColWidth(summarySheet, "A", "A", 34)
	_ = file.SetColWidth(summarySheet, "B", "B", 20)
	_ = file.SetColWidth(summarySheet, "C", "D", 20)

	categoryTotals := map[string]struct {
		Count int
		Net   float64
	}{}
	for _, exp := range expenses {
		key := strings.TrimSpace(exp.Category)
		if key == "" {
			key = "Sin categoria"
		}
		item := categoryTotals[key]
		item.Count++
		item.Net += exp.Amount
		categoryTotals[key] = item
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
		if math.Abs(categoryRows[i].Net) == math.Abs(categoryRows[j].Net) {
			return categoryRows[i].Name < categoryRows[j].Name
		}
		return math.Abs(categoryRows[i].Net) > math.Abs(categoryRows[j].Net)
	})

	_ = file.SetCellValue(categoriesSheet, "A1", "Categorias destacadas")
	_ = file.MergeCell(categoriesSheet, "A1", "C1")
	_ = file.SetCellStyle(categoriesSheet, "A1", "C1", summaryTitleStyle)

	_ = file.SetCellValue(categoriesSheet, "A3", "Categoria")
	_ = file.SetCellValue(categoriesSheet, "B3", "Cantidad")
	_ = file.SetCellValue(categoriesSheet, "C3", "Monto neto")
	_ = file.SetCellStyle(categoriesSheet, "A3", "C3", headerStyle)

	for idx, row := range categoryRows {
		r := idx + 4
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("A%d", r), row.Name)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("B%d", r), row.Count)
		_ = file.SetCellValue(categoriesSheet, fmt.Sprintf("C%d", r), row.Net)
		startCell := fmt.Sprintf("A%d", r)
		endCell := fmt.Sprintf("C%d", r)
		amountCell := fmt.Sprintf("C%d", r)
		if idx%2 == 0 {
			_ = file.SetCellStyle(categoriesSheet, startCell, endCell, bodyStyle)
			_ = file.SetCellStyle(categoriesSheet, amountCell, amountCell, amountStyle)
		} else {
			_ = file.SetCellStyle(categoriesSheet, startCell, endCell, oddRowStyle)
			_ = file.SetCellStyle(categoriesSheet, amountCell, amountCell, amountOddStyle)
		}
	}
	_ = file.SetColWidth(categoriesSheet, "A", "A", 30)
	_ = file.SetColWidth(categoriesSheet, "B", "B", 14)
	_ = file.SetColWidth(categoriesSheet, "C", "C", 18)

	file.SetActiveSheet(1)

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
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-9)
		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(0, 5, fmt.Sprintf("ExpenseLog | Pagina %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()

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
	pdf.SetXY(pageLeft+3, topY+18)
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
			Title:      "Movimientos",
			Value:      fmt.Sprintf("%d", metrics.TransactionCount),
			FillColor:  [3]int{239, 246, 255},
			LineColor:  [3]int{147, 197, 253},
			TitleColor: [3]int{30, 64, 175},
		},
		{
			Title:      "Ingresos",
			Value:      formatReportAmount(metrics.Income, query.Currency),
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
			FillColor:  [3]int{255, 251, 235},
			LineColor:  [3]int{253, 186, 116},
			TitleColor: [3]int{146, 64, 14},
		},
		{
			Title:      "Tarjeta por pagar",
			Value:      formatReportAmount(metrics.CardPending, query.Currency),
			FillColor:  [3]int{238, 242, 255},
			LineColor:  [3]int{165, 180, 252},
			TitleColor: [3]int{55, 48, 163},
		},
	}

	cardW := 91.0
	cardH := 16.0
	cardGap := 8.0
	startCardY := 31.0
	for idx, card := range cards {
		x := pageLeft
		y := startCardY
		width := cardW
		if idx < 4 {
			col := idx % 2
			row := idx / 2
			x = pageLeft + float64(col)*(cardW+cardGap)
			y = startCardY + float64(row)*(cardH+5)
		} else {
			// Last card spans full width to avoid awkward empty space.
			x = pageLeft
			y = startCardY + 2*(cardH+5)
			width = pageWidth
		}

		pdf.SetDrawColor(card.LineColor[0], card.LineColor[1], card.LineColor[2])
		pdf.SetFillColor(card.FillColor[0], card.FillColor[1], card.FillColor[2])
		pdf.RoundedRect(x, y, width, cardH, 2, "1234", "DF")
		pdf.SetXY(x+3, y+3)
		pdf.SetTextColor(card.TitleColor[0], card.TitleColor[1], card.TitleColor[2])
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(width-6, 4, card.Title, "", 2, "L", false, 0, "")
		pdf.SetX(x + 3)
		pdf.SetTextColor(15, 23, 42)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(width-6, 5, card.Value, "", 0, "L", false, 0, "")
	}

	pdf.SetY(startCardY + 3*(cardH+5))
	pdf.Ln(1)
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
			trimForPDF(formatFlowLabel(exp.Flow), 14),
			trimForPDF(exp.Category, 18),
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

	buffer := bytes.NewBuffer(nil)
	if err := pdf.Output(buffer); err != nil {
		return nil, err
	}
	return buffer, nil
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
