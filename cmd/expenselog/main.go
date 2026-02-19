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

	// Version Handler
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(version))
	})

	// UI Handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
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
		w.Header().Set("Content-Type", "text/html")
		if err := web.ServeTemplate(w, "index.html"); err != nil {
			log.Printf("HTTP ERROR: Failed to serve template: %v", err)
			http.Error(w, "Failed to serve template", http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/table", handler.ServeTableView)
	http.HandleFunc("/settings", handler.ServeSettingsPage)

	// Static File Handlers
	http.HandleFunc("/robots.txt", handler.ServeStaticFile)
	http.HandleFunc("/sitemap.xml", handler.ServeStaticFile)
	http.HandleFunc("/functions.js", handler.ServeStaticFile)
	http.HandleFunc("/alerts_ui.js", handler.ServeStaticFile)
	http.HandleFunc("/cashflow_ui.js", handler.ServeStaticFile)
	http.HandleFunc("/manifest.json", handler.ServeStaticFile)
	http.HandleFunc("/sw.js", handler.ServeStaticFile)
	http.HandleFunc("/pwa/", handler.ServeStaticFile)
	http.HandleFunc("/style.css", handler.ServeStaticFile)
	http.HandleFunc("/favicon.ico", handler.ServeStaticFile)
	http.HandleFunc("/chart.min.js", handler.ServeStaticFile)
	http.HandleFunc("/fa.min.css", handler.ServeStaticFile)
	http.HandleFunc("/webfonts/", handler.ServeStaticFile)

	// Auth
	http.HandleFunc("/auth/register", handler.AuthRegister)
	http.HandleFunc("/auth/login", handler.AuthLogin)
	http.HandleFunc("/auth/logout", handler.AuthLogout)
	http.HandleFunc("/auth/me", handler.RequireAuth(handler.AuthMe))
	http.HandleFunc("/auth/profile", handler.RequireAuth(handler.AuthUpdateProfile))
	http.HandleFunc("/auth/reset/request", handler.AuthResetRequest)
	http.HandleFunc("/auth/reset/confirm", handler.AuthResetConfirm)
	http.HandleFunc("/auth/verify", handler.AuthVerifyEmail)
	http.HandleFunc("/auth/google", handler.AuthGoogleStart)
	http.HandleFunc("/auth/google/callback", handler.AuthGoogleCallback)

	// Config
	http.HandleFunc("/config", handler.RequireAuth(handler.GetConfig))
	http.HandleFunc("/categories", handler.RequireAuth(handler.GetCategories))
	http.HandleFunc("/categories/edit", handler.RequireAuth(handler.UpdateCategories))
	http.HandleFunc("/categories/add", handler.RequireAuth(handler.AddCategory))
	http.HandleFunc("/categories/rename", handler.RequireAuth(handler.RenameCategory))
	http.HandleFunc("/categories/delete", handler.RequireAuth(handler.DeleteCategory))
	http.HandleFunc("/currency", handler.RequireAuth(handler.GetCurrency))
	http.HandleFunc("/currency/edit", handler.RequireAuth(handler.UpdateCurrency))
	http.HandleFunc("/startdate", handler.RequireAuth(handler.GetStartDate))
	http.HandleFunc("/startdate/edit", handler.RequireAuth(handler.UpdateStartDate))
	// http.HandleFunc("/tags", handler.GetTags)
	// http.HandleFunc("/tags/edit", handler.UpdateTags)

	// Reconciliation
	http.HandleFunc("/reconciliation/apply", handler.RequireAuth(handler.ReconcileBalance))
	http.HandleFunc("/reconciliation/history", handler.RequireAuth(handler.GetReconciliationHistory))
	http.HandleFunc("/reconciliation/revert", handler.RequireAuth(handler.RevertReconciliation))

	// Expenses
	http.HandleFunc("/expense", handler.RequireAuth(handler.AddExpense))                     // PUT for add
	http.HandleFunc("/expenses", handler.RequireAuth(handler.GetExpenses))                   // GET all
	http.HandleFunc("/expense/edit", handler.RequireAuth(handler.EditExpense))               // PUT for edit
	http.HandleFunc("/expense/delete", handler.RequireAuth(handler.DeleteExpense))           // DELETE for single
	http.HandleFunc("/expenses/delete", handler.RequireAuth(handler.DeleteMultipleExpenses)) // DELETE for multiple

	// Recurring Expenses
	http.HandleFunc("/recurring-expense", handler.RequireAuth(handler.AddRecurringExpense))           // PUT for add
	http.HandleFunc("/recurring-expenses", handler.RequireAuth(handler.GetRecurringExpenses))         // GET all
	http.HandleFunc("/recurring-expense/edit", handler.RequireAuth(handler.UpdateRecurringExpense))   // PUT for edit
	http.HandleFunc("/recurring-expense/delete", handler.RequireAuth(handler.DeleteRecurringExpense)) // DELETE

	// Alerts
	http.HandleFunc("/alerts/liquidity", handler.RequireAuth(handler.GetLiquidityAlerts))

	// Import/Export
	http.HandleFunc("/export/csv", handler.RequireAuth(handler.CSVFeatureDisabled))
	http.HandleFunc("/import/csv", handler.RequireAuth(handler.CSVFeatureDisabled))
	http.HandleFunc("/import/csvold", handler.RequireAuth(handler.CSVFeatureDisabled))

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
