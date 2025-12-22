package handlers

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// ToolsHandler handles tools/calculator routes.
type ToolsHandler struct {
	templates       map[string]*template.Template
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	categoryRepo    *repository.CategoryRepository
}

// NewToolsHandler creates a new ToolsHandler.
func NewToolsHandler(
	templates map[string]*template.Template,
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
	categoryRepo *repository.CategoryRepository,
) *ToolsHandler {
	return &ToolsHandler{
		templates:       templates,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
	}
}

// List renders the tools listing page.
func (h *ToolsHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "tools.html", map[string]any{
		"Title":     "Tools",
		"User":      user,
		"ActiveNav": "tools",
	})
}

// CompoundInterest renders the compound interest calculator.
func (h *ToolsHandler) CompoundInterest(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "compound-interest.html", map[string]any{
		"Title":     "Compound Interest Calculator",
		"User":      user,
		"ActiveNav": "tools",
	})
}

// SalaryCalculator renders the Danish salary calculator.
func (h *ToolsHandler) SalaryCalculator(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "salary-calculator.html", map[string]any{
		"Title":     "Salary Calculator",
		"User":      user,
		"ActiveNav": "tools",
	})
}

// FIRECalculator renders the Danish FIRE (Financial Independence, Retire Early) calculator.
func (h *ToolsHandler) FIRECalculator(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Fetch accounts and categories for pre-fill
	accounts, _ := h.accountRepo.GetByUserIDActiveOnly(user.ID)
	categories, _ := h.categoryRepo.GetByUserID(user.ID)

	// Calculate totals by FIRE account type
	accountData := h.calculateFIREAccountTotals(accounts, categories)

	// Convert to JSON for Alpine.js
	accountDataJSON, err := json.Marshal(accountData)
	if err != nil {
		accountDataJSON = []byte("{}")
	}

	h.render(w, "fire-calculator.html", map[string]any{
		"Title":       "FIRE Calculator (Denmark)",
		"User":        user,
		"ActiveNav":   "tools",
		"AccountData": template.JS(accountDataJSON),
	})
}

// calculateFIREAccountTotals categorizes account balances into FIRE account types.
func (h *ToolsHandler) calculateFIREAccountTotals(accounts []*models.Account, categories []*models.Category) map[string]float64 {
	// Create category lookup by ID
	categoryMap := make(map[int64]string)
	for _, cat := range categories {
		categoryMap[cat.ID] = strings.ToLower(cat.Name)
	}

	totals := map[string]float64{
		"frieMidler":  0,
		"ask":         0,
		"pension":     0,
		"liabilities": 0,
	}

	for _, acc := range accounts {
		if !acc.IsActive {
			continue
		}

		// Handle liabilities separately - use absolute value since balances may be stored negative
		if acc.IsLiability {
			balance, err := h.transactionRepo.GetLatestBalance(acc.ID)
			if err == nil {
				if balance < 0 {
					balance = -balance // Convert to positive
				}
				totals["liabilities"] += balance
			}
			continue
		}

		// Get latest balance for this account
		balance, err := h.transactionRepo.GetLatestBalance(acc.ID)
		if err != nil {
			continue
		}

		// Determine category name
		catName := ""
		if acc.CategoryID != nil {
			catName = categoryMap[*acc.CategoryID]
		}

		// Also check account name for classification
		accNameLower := strings.ToLower(acc.Name)

		// Categorize based on category name or account name keywords
		switch {
		case strings.Contains(catName, "ask") || strings.Contains(catName, "aktiesparekonto") ||
			strings.Contains(accNameLower, "ask") || strings.Contains(accNameLower, "aktiesparekonto"):
			totals["ask"] += balance
		case strings.Contains(catName, "pension") || strings.Contains(accNameLower, "pension") ||
			strings.Contains(accNameLower, "aldersopsparing") || strings.Contains(accNameLower, "ratepension"):
			totals["pension"] += balance
		default:
			// Stocks, crypto, savings, cash = frie midler
			totals["frieMidler"] += balance
		}
	}

	return totals
}

// render renders a template with the given data.
func (h *ToolsHandler) render(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("Error rendering template %s: %v", name, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}
