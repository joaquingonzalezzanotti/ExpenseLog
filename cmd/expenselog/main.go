package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/api"
	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/storage"
	"github.com/joaquingonzalezzanotti/ExpenseLog/internal/web"
)

var version = "dev"

func runServer(port int) {
	storage, err := storage.InitializeStorage()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()
	handler := api.NewHandler(storage)

	registerAPI := func(path string, h http.HandlerFunc) {
		http.HandleFunc(path, h)
		http.HandleFunc("/api"+path, h)
	}

	// Version Handler
	versionHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(version))
	}
	http.HandleFunc("/version", versionHandler)
	http.HandleFunc("/api/version", versionHandler)

	// UI Handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := web.ServeTemplate(w, "landing.html"); err != nil {
			log.Printf("HTTP ERROR: Failed to serve template: %v", err)
			http.Error(w, "Failed to serve template", http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := web.ServeTemplate(w, "index.html"); err != nil {
			log.Printf("HTTP ERROR: Failed to serve template: %v", err)
			http.Error(w, "Failed to serve template", http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/" || r.URL.Path == "/app/index" || r.URL.Path == "/app/index.html" {
			http.Redirect(w, r, "/app", http.StatusPermanentRedirect)
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/app/table", handler.ServeTableView)
	http.HandleFunc("/app/settings", handler.ServeSettingsPage)
	http.HandleFunc("/app/perfil", handler.ServeSettingsPage)
	http.HandleFunc("/app/categorias", handler.ServeSettingsCategoriesPage)
	http.HandleFunc("/app/recurrentes", handler.ServeSettingsRecurringPage)
	http.HandleFunc("/app/conciliacion", handler.ServeSettingsReconciliationPage)
	http.HandleFunc("/app/reportes", handler.ServeSettingsReportsPage)
	http.HandleFunc("/app/telegram", handler.ServeSettingsTelegramPage)
	http.HandleFunc("/table", handler.ServeTableView)
	http.HandleFunc("/settings", handler.ServeSettingsPage)
	http.HandleFunc("/perfil", handler.ServeSettingsPage)
	http.HandleFunc("/categorias", handler.ServeSettingsCategoriesPage)
	http.HandleFunc("/recurrentes", handler.ServeSettingsRecurringPage)
	http.HandleFunc("/conciliacion", handler.ServeSettingsReconciliationPage)
	http.HandleFunc("/reportes", handler.ServeSettingsReportsPage)
	http.HandleFunc("/telegram", handler.ServeSettingsTelegramPage)

	// Static File Handlers
	staticPaths := []string{
		"/robots.txt",
		"/sitemap.xml",
		"/functions.js",
		"/alerts_ui.js",
		"/cashflow_ui.js",
		"/manifest.json",
		"/sw.js",
		"/pwa/",
		"/style.css",
		"/favicon.ico",
		"/chart.min.js",
		"/fa.min.css",
		"/webfonts/",
	}
	for _, path := range staticPaths {
		http.HandleFunc(path, handler.ServeStaticFile)
		http.HandleFunc("/app"+path, handler.ServeStaticFile)
	}

	// Auth
	registerAPI("/auth/register", handler.AuthRegister)
	registerAPI("/auth/login", handler.AuthLogin)
	registerAPI("/auth/logout", handler.AuthLogout)
	registerAPI("/auth/me", handler.RequireAuth(handler.AuthMe))
	registerAPI("/auth/profile", handler.RequireAuth(handler.AuthUpdateProfile))
	registerAPI("/auth/reset/request", handler.AuthResetRequest)
	registerAPI("/auth/reset/confirm", handler.AuthResetConfirm)
	registerAPI("/auth/verify", handler.AuthVerifyEmail)
	registerAPI("/auth/google", handler.AuthGoogleStart)
	registerAPI("/auth/google/callback", handler.AuthGoogleCallback)

	// Config
	registerAPI("/config", handler.RequireAuth(handler.GetConfig))
	registerAPI("/categories", handler.RequireAuth(handler.GetCategories))
	registerAPI("/categories/edit", handler.RequireAuth(handler.UpdateCategories))
	registerAPI("/categories/add", handler.RequireAuth(handler.AddCategory))
	registerAPI("/categories/rename", handler.RequireAuth(handler.RenameCategory))
	registerAPI("/categories/delete", handler.RequireAuth(handler.DeleteCategory))
	registerAPI("/currency", handler.RequireAuth(handler.GetCurrency))
	registerAPI("/currency/edit", handler.RequireAuth(handler.UpdateCurrency))
	registerAPI("/startdate", handler.RequireAuth(handler.GetStartDate))
	registerAPI("/startdate/edit", handler.RequireAuth(handler.UpdateStartDate))
	registerAPI("/telegram/link-status", handler.RequireAuth(handler.GetTelegramLinkStatus))
	registerAPI("/telegram/link-code", handler.RequireAuth(handler.CreateTelegramLinkCode))
	registerAPI("/telegram/refresh-status", handler.RequireAuth(handler.RefreshTelegramLinkStatus))
	// http.HandleFunc("/tags", handler.GetTags)
	// http.HandleFunc("/tags/edit", handler.UpdateTags)

	// Reconciliation
	registerAPI("/reconciliation/apply", handler.RequireAuth(handler.ReconcileBalance))
	registerAPI("/reconciliation/history", handler.RequireAuth(handler.GetReconciliationHistory))
	registerAPI("/reconciliation/revert", handler.RequireAuth(handler.RevertReconciliation))

	// Expenses
	registerAPI("/expense", handler.RequireAuth(handler.AddExpense))                     // PUT for add
	registerAPI("/expenses", handler.RequireAuth(handler.GetExpenses))                   // GET all
	registerAPI("/expense/edit", handler.RequireAuth(handler.EditExpense))               // PUT for edit
	registerAPI("/expense/delete", handler.RequireAuth(handler.DeleteExpense))           // DELETE for single
	registerAPI("/expenses/delete", handler.RequireAuth(handler.DeleteMultipleExpenses)) // DELETE for multiple
	registerAPI("/card/payment", handler.RequireAuth(handler.AddCardPayment))            // POST for card payment events
	registerAPI("/integrations/apple-wallet/debug", handler.RequireAuth(handler.AppleWalletDebug))
	registerAPI("/integrations/apple-wallet/token-status", handler.RequireAuth(handler.GetAppleWalletIngestTokenStatus))
	registerAPI("/integrations/apple-wallet/token", handler.RequireAuth(handler.CreateAppleWalletIngestToken))
	registerAPI("/integrations/apple-wallet/ingest", handler.AppleWalletIngest)

	// Recurring Expenses
	registerAPI("/recurring-expense", handler.RequireAuth(handler.AddRecurringExpense))           // PUT for add
	registerAPI("/recurring-expenses", handler.RequireAuth(handler.GetRecurringExpenses))         // GET all
	registerAPI("/recurring-expense/edit", handler.RequireAuth(handler.UpdateRecurringExpense))   // PUT for edit
	registerAPI("/recurring-expense/delete", handler.RequireAuth(handler.DeleteRecurringExpense)) // DELETE

	// Alerts
	registerAPI("/alerts/liquidity", handler.RequireAuth(handler.GetLiquidityAlerts))

	// Import/Export
	registerAPI("/export/csv", handler.RequireAuth(handler.CSVFeatureDisabled))
	registerAPI("/export/monthly/xlsx", handler.RequireAuth(handler.ExportMonthlyXLSX))
	registerAPI("/export/monthly/pdf", handler.RequireAuth(handler.ExportMonthlyPDF))
	registerAPI("/import/csv", handler.RequireAuth(handler.CSVFeatureDisabled))
	registerAPI("/import/csvold", handler.RequireAuth(handler.CSVFeatureDisabled))

	// Telegram bot internal API
	registerAPI("/bot/telegram/consume-link-code", handler.RequireBotAuth(handler.ConsumeTelegramLinkCode))
	registerAPI("/bot/telegram/link-status", handler.RequireBotAuth(handler.GetBotTelegramLinkStatus))
	registerAPI("/bot/expense", handler.RequireBotAuth(handler.CreateBotExpense))

	server := &http.Server{
		Addr:              fmt.Sprint(":", port),
		Handler:           withSecurityHeaders(http.DefaultServeMux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("Starting server on port", port, "...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		if isHTTPSRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		return false
	}
	parts := strings.Split(proto, ",")
	return strings.EqualFold(strings.TrimSpace(parts[0]), "https")
}

func main() {
	port := flag.Int("port", 8080, "Port to serve from")
	flag.Parse()
	runServer(*port)
}
