package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/config"
	"wealth_tracker/internal/database"
	"wealth_tracker/internal/demo"
	"wealth_tracker/internal/handlers"
	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
	"wealth_tracker/internal/services"
	"wealth_tracker/internal/sync"
)

// App holds the application dependencies.
type App struct {
	config             *config.Config
	db                 *database.DB
	templates          TemplateCache
	router             *chi.Mux
	userRepo           *repository.UserRepository
	categoryRepo       *repository.CategoryRepository
	accountRepo        *repository.AccountRepository
	transactionRepo    *repository.TransactionRepository
	goalRepo           *repository.GoalRepository
	brokerConnRepo     *repository.BrokerConnectionRepository
	holdingRepo        *repository.HoldingRepository
	mappingRepo        *repository.AccountMappingRepository
	syncHistoryRepo    *repository.SyncHistoryRepository
	sessionManager     *auth.SessionManager
	authMiddleware     *middleware.AuthMiddleware
	authHandler        *handlers.AuthHandler
	dashHandler        *handlers.DashboardHandler
	categoryHandler    *handlers.CategoryHandler
	accountHandler     *handlers.AccountHandler
	transactionHandler *handlers.TransactionHandler
	goalHandler        *handlers.GoalHandler
	settingsHandler    *handlers.SettingsHandler
	toolsHandler       *handlers.ToolsHandler
	adminHandler       *handlers.AdminHandler
	exportHandler      *handlers.ExportHandler
	brokerHandler      *handlers.BrokerHandler
	portfolioHandler   *handlers.PortfolioHandler
}

func main() {
	// Load configuration
	cfg := config.New()

	// Initialize database
	db, err := database.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Create repositories early for admin check
	userRepo := repository.NewUserRepository(db)

	// In demo mode, seed demo data; otherwise create default admin
	if cfg.DemoMode {
		seeder := demo.NewSeeder(db)
		if err := seeder.SeedIfEmpty(); err != nil {
			log.Fatalf("Failed to seed demo data: %v", err)
		}
	} else {
		// Create default admin if no users exist
		if err := ensureDefaultAdmin(userRepo); err != nil {
			log.Fatalf("Failed to ensure default admin: %v", err)
		}
	}

	// Parse templates
	templates, err := parseTemplates()
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Create repositories (userRepo already created above for admin check)
	categoryRepo := repository.NewCategoryRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	goalRepo := repository.NewGoalRepository(db)
	brokerConnRepo := repository.NewBrokerConnectionRepository(db)
	holdingRepo := repository.NewHoldingRepository(db)
	mappingRepo := repository.NewAccountMappingRepository(db)
	syncHistoryRepo := repository.NewSyncHistoryRepository(db)
	allocationTargetRepo := repository.NewAllocationTargetRepository(db)

	// Get scripts directory for MitID authentication
	workDir, _ := os.Getwd()
	scriptDir := filepath.Join(workDir, "scripts")

	// Create sync service
	syncService := sync.NewService(brokerConnRepo, holdingRepo, mappingRepo, syncHistoryRepo, transactionRepo, scriptDir)

	// Create portfolio service
	portfolioService := services.NewPortfolioService(accountRepo, holdingRepo, categoryRepo, transactionRepo, allocationTargetRepo)

	// Create session manager
	sessionManager := auth.NewSessionManager(db)

	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(sessionManager, userRepo)

	// Create handlers
	authHandler := handlers.NewAuthHandler(templates, userRepo, sessionManager)
	dashHandler := handlers.NewDashboardHandler(templates, accountRepo, transactionRepo, goalRepo, categoryRepo)
	categoryHandler := handlers.NewCategoryHandler(templates, categoryRepo, accountRepo)
	accountHandler := handlers.NewAccountHandler(templates, accountRepo, categoryRepo, transactionRepo, holdingRepo)
	transactionHandler := handlers.NewTransactionHandler(templates, transactionRepo, accountRepo, categoryRepo)
	goalHandler := handlers.NewGoalHandler(templates, goalRepo, accountRepo, transactionRepo, categoryRepo)
	settingsHandler := handlers.NewSettingsHandler(templates, userRepo)
	toolsHandler := handlers.NewToolsHandler(templates, accountRepo, transactionRepo, categoryRepo)
	adminHandler := handlers.NewAdminHandler(templates, db, userRepo, accountRepo, categoryRepo, goalRepo, transactionRepo, sessionManager)
	exportHandler := handlers.NewExportHandler(accountRepo, transactionRepo, categoryRepo, goalRepo)
	brokerHandler := handlers.NewBrokerHandler(templates, brokerConnRepo, mappingRepo, holdingRepo, syncHistoryRepo, accountRepo, syncService)
	portfolioHandler := handlers.NewPortfolioHandler(templates, portfolioService, allocationTargetRepo, categoryRepo)

	// Create application
	app := &App{
		config:             cfg,
		db:                 db,
		templates:          templates,
		userRepo:           userRepo,
		categoryRepo:       categoryRepo,
		accountRepo:        accountRepo,
		transactionRepo:    transactionRepo,
		goalRepo:           goalRepo,
		brokerConnRepo:     brokerConnRepo,
		holdingRepo:        holdingRepo,
		mappingRepo:        mappingRepo,
		syncHistoryRepo:    syncHistoryRepo,
		sessionManager:     sessionManager,
		authMiddleware:     authMiddleware,
		authHandler:        authHandler,
		dashHandler:        dashHandler,
		categoryHandler:    categoryHandler,
		accountHandler:     accountHandler,
		transactionHandler: transactionHandler,
		goalHandler:        goalHandler,
		settingsHandler:    settingsHandler,
		toolsHandler:       toolsHandler,
		adminHandler:       adminHandler,
		exportHandler:      exportHandler,
		brokerHandler:      brokerHandler,
		portfolioHandler:   portfolioHandler,
	}

	// Setup router
	app.setupRouter()

	// Create server
	server := &http.Server{
		Addr:         cfg.Address(),
		Handler:      app.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on http://%s", cfg.Address())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

func (app *App) setupRouter() {
	r := chi.NewRouter()

	// Chi middleware (aliased as chimw to avoid conflict with our middleware package)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(chimw.Compress(5))

	// Security headers for all responses
	r.Use(middleware.SecurityHeaders)

	// Load user from session for all routes
	r.Use(app.authMiddleware.LoadUser)

	// Static files
	workDir, _ := os.Getwd()
	staticPath := filepath.Join(workDir, "web", "static")
	fileServer := http.FileServer(http.Dir(staticPath))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Health check
	r.Get("/health", app.handleHealth)

	// Public routes (redirect if already authenticated)
	// Rate limited to prevent brute force attacks
	r.Group(func(r chi.Router) {
		r.Use(app.authMiddleware.RedirectIfAuthenticated)
		r.Use(middleware.LimitAuth)
		r.Get("/login", app.authHandler.LoginPage)
		r.Post("/login", app.authHandler.Login)
		r.Get("/register", app.authHandler.RegisterPage)
		r.Post("/register", app.authHandler.Register)
	})

	// Change password route (requires auth but NOT password changed)
	// Rate limited to prevent password guessing
	r.Group(func(r chi.Router) {
		r.Use(app.authMiddleware.RequireAuth)
		r.Use(middleware.LimitAuth)
		r.Get("/change-password", app.authHandler.ChangePasswordPage)
		r.Post("/change-password", app.authHandler.ChangePassword)
	})

	// Protected routes (require authentication AND password changed)
	r.Group(func(r chi.Router) {
		r.Use(app.authMiddleware.RequireAuth)
		r.Use(app.authMiddleware.RequirePasswordChanged)
		r.Get("/dashboard", app.dashHandler.Dashboard)

		// Categories
		r.Get("/categories", app.categoryHandler.List)
		r.Post("/categories", app.categoryHandler.Create)
		r.Post("/categories/{id}", app.categoryHandler.Update)

		// Accounts
		r.Get("/accounts", app.accountHandler.List)
		r.Post("/accounts", app.accountHandler.Create)
		r.Post("/accounts/{id}", app.accountHandler.Update)
		r.Post("/accounts/{id}/balance", app.accountHandler.UpdateBalance)

		// Transactions
		r.Get("/transactions", app.transactionHandler.List)
		r.Post("/transactions", app.transactionHandler.Create)
		r.Post("/transactions/{id}", app.transactionHandler.Update)

		// Goals
		r.Get("/goals", app.goalHandler.List)
		r.Post("/goals", app.goalHandler.Create)
		r.Post("/goals/{id}", app.goalHandler.Update)

		// Settings
		r.Get("/settings", app.settingsHandler.Settings)
		r.Post("/settings", app.settingsHandler.Update)

		// Broker Connections
		r.Get("/settings/connections", app.brokerHandler.Connections)
		r.Get("/settings/connections/new", app.brokerHandler.NewConnectionForm)
		r.Post("/settings/connections", app.brokerHandler.CreateConnection)
		r.Get("/settings/connections/{id}", app.brokerHandler.ViewConnection)
		r.Get("/settings/connections/{id}/edit", app.brokerHandler.EditConnectionForm)
		r.Post("/settings/connections/{id}/edit", app.brokerHandler.UpdateConnection)
		r.Get("/settings/connections/{id}/accounts", app.brokerHandler.AccountMappingForm)
		r.Post("/settings/connections/{id}/accounts", app.brokerHandler.SaveAccountMappings)
		r.Post("/settings/connections/{id}/fetch-accounts", app.brokerHandler.FetchExternalAccounts)
		r.Post("/settings/connections/{id}/sync", app.brokerHandler.SyncConnection)
		r.Post("/settings/connections/{id}/delete", app.brokerHandler.DeleteConnection)
		r.Get("/settings/connections/{id}/mitid/status", app.brokerHandler.MitIDStatus)
		r.Get("/settings/connections/{id}/mitid/qr", app.brokerHandler.MitIDQRCode)
		// Saxo OAuth
		r.Get("/settings/connections/{id}/saxo/status", app.brokerHandler.SaxoOAuthStatus)
		r.Post("/settings/connections/{id}/saxo/auth", app.brokerHandler.SaxoStartOAuth)

		// Tools
		r.Get("/tools", app.toolsHandler.List)
		r.Get("/tools/compound-interest", app.toolsHandler.CompoundInterest)
		r.Get("/tools/salary-calculator", app.toolsHandler.SalaryCalculator)
		r.Get("/tools/fire-calculator", app.toolsHandler.FIRECalculator)
		r.Get("/tools/portfolio-analyzer", app.portfolioHandler.Analyzer)

		// Portfolio API
		r.Get("/api/portfolio/composition", app.portfolioHandler.GetComposition)
		r.Get("/api/portfolio/targets", app.portfolioHandler.GetTargets)
		r.Post("/api/portfolio/targets", app.portfolioHandler.SaveTarget)
		r.Delete("/api/portfolio/targets", app.portfolioHandler.DeleteTarget)
		r.Get("/api/portfolio/comparison", app.portfolioHandler.GetComparison)
		r.Get("/api/portfolio/rebalance", app.portfolioHandler.GetRebalancing)

		// Export
		r.Get("/export/transactions", app.exportHandler.ExportTransactions)
		r.Get("/export/accounts", app.exportHandler.ExportAccounts)
		r.Get("/export/all", app.exportHandler.ExportAll)
	})

	// Admin return route (accessible when impersonating - only requires auth)
	r.Group(func(r chi.Router) {
		r.Use(app.authMiddleware.RequireAuth)
		r.Post("/admin/return", app.adminHandler.ReturnFromImpersonation)
	})

	// Admin routes (require authentication, password changed, and admin status)
	r.Group(func(r chi.Router) {
		r.Use(app.authMiddleware.RequireAuth)
		r.Use(app.authMiddleware.RequirePasswordChanged)
		r.Use(app.authMiddleware.RequireAdmin)

		r.Get("/admin", app.adminHandler.Dashboard)
		r.Get("/admin/users", app.adminHandler.UserList)
		r.Get("/admin/users/{id}", app.adminHandler.UserView)
		r.Post("/admin/users/{id}", app.adminHandler.UserEdit)
		r.Post("/admin/users/{id}/reset-password", app.adminHandler.UserResetPassword)
		r.Post("/admin/users/{id}/delete", app.adminHandler.UserDelete)
		r.Post("/admin/users/{id}/impersonate", app.adminHandler.UserImpersonate)

		r.Get("/admin/database", app.adminHandler.DatabaseOverview)
		r.Get("/admin/database/{table}", app.adminHandler.TableView)
		r.Get("/admin/database/{table}/{id}", app.adminHandler.TableRowView)
		r.Get("/admin/sql", app.adminHandler.SQLQueryPage)
		r.Post("/admin/sql", app.adminHandler.SQLQueryExecute)
	})

	// Logout (needs to be accessible when logged in)
	r.Post("/logout", app.authHandler.Logout)

	// Index route - redirect based on auth status
	r.Get("/", app.handleIndex)

	app.router = r
}

// handleHealth returns the server health status.
func (app *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleIndex redirects to dashboard or login based on auth status.
func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// TemplateCache holds parsed templates.
type TemplateCache map[string]*template.Template

// parseTemplates loads and parses all templates.
func parseTemplates() (TemplateCache, error) {
	cache := make(TemplateCache)

	// Template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"subtract": func(a, b int) int {
			return a - b
		},
		// formatNumber formats a number according to locale
		// format: "da" (Danish: 1.234,56), "en" (English: 1,234.56), "fr" (French: 1 234,56)
		"formatNumber": func(n float64, format string) string {
			return formatNumberWithLocale(n, format, false)
		},
		// formatNumberDecimals formats a number with 2 decimal places
		"formatNumberDecimals": func(n float64, format string) string {
			return formatNumberWithLocale(n, format, true)
		},
		// upper converts a string to uppercase
		"upper": func(s string) string {
			return strings.ToUpper(s)
		},
	}

	// Get layout path
	layoutPath := filepath.Join("web", "templates", "layouts", "base.html")

	// Get all page templates
	pagesGlob := filepath.Join("web", "templates", "pages", "*.html")
	pages, err := filepath.Glob(pagesGlob)
	if err != nil {
		return nil, err
	}

	// Parse each page with the layout
	for _, page := range pages {
		name := filepath.Base(page)

		// Parse layout first, then page (with functions)
		tmpl, err := template.New(filepath.Base(layoutPath)).Funcs(funcMap).ParseFiles(layoutPath, page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}

		cache[name] = tmpl
	}

	return cache, nil
}

// formatNumberWithLocale formats a number according to the specified locale.
// Supported formats: "da" (Danish), "en" (English), "fr" (French)
func formatNumberWithLocale(n float64, format string, withDecimals bool) string {
	// Handle negative numbers
	negative := n < 0
	if negative {
		n = -n
	}

	var intPart int64
	var decPart int
	if withDecimals {
		intPart = int64(n)
		decPart = int((n - float64(intPart)) * 100)
	} else {
		intPart = int64(n + 0.5) // Round to nearest
	}

	// Format the integer part with thousand separators
	intStr := formatIntWithSeparator(intPart, format)

	var result string
	if withDecimals {
		decSep := ","
		if format == "en" {
			decSep = "."
		}
		result = fmt.Sprintf("%s%s%02d", intStr, decSep, decPart)
	} else {
		result = intStr
	}

	if negative {
		result = "-" + result
	}

	return result
}

// formatIntWithSeparator formats an integer with thousand separators.
func formatIntWithSeparator(n int64, format string) string {
	// Determine separator based on format
	sep := "."
	switch format {
	case "en":
		sep = ","
	case "fr":
		sep = " "
	case "da", "de":
		sep = "."
	}

	// Convert to string and add separators
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	// Add separators every 3 digits from the right
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, sep[0])
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// ensureDefaultAdmin creates a default admin user if no users exist.
// The default admin must change their password before others can register.
func ensureDefaultAdmin(userRepo *repository.UserRepository) error {
	count, err := userRepo.CountAll()
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}

	if count > 0 {
		return nil // Users exist, nothing to do
	}

	// Create default admin
	passwordHash, err := auth.HashPassword("changeme")
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	admin := &models.User{
		Email:              "admin@localhost",
		PasswordHash:       passwordHash,
		Name:               "Admin",
		DefaultCurrency:    "DKK",
		NumberFormat:       "da",
		Theme:              "dark",
		IsAdmin:            true,
		MustChangePassword: true,
	}

	_, err = userRepo.Create(admin)
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	log.Println("========================================")
	log.Println("DEFAULT ADMIN CREATED")
	log.Println("Email:    admin@localhost")
	log.Println("Password: changeme")
	log.Println("You MUST change this password on first login!")
	log.Println("========================================")

	return nil
}
