package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// AccountHandler handles account routes.
type AccountHandler struct {
	templates       map[string]*template.Template
	accountRepo     *repository.AccountRepository
	categoryRepo    *repository.CategoryRepository
	transactionRepo *repository.TransactionRepository
	holdingRepo     *repository.HoldingRepository
}

// NewAccountHandler creates a new AccountHandler.
func NewAccountHandler(
	templates map[string]*template.Template,
	accountRepo *repository.AccountRepository,
	categoryRepo *repository.CategoryRepository,
	transactionRepo *repository.TransactionRepository,
	holdingRepo *repository.HoldingRepository,
) *AccountHandler {
	return &AccountHandler{
		templates:       templates,
		accountRepo:     accountRepo,
		categoryRepo:    categoryRepo,
		transactionRepo: transactionRepo,
		holdingRepo:     holdingRepo,
	}
}

// List renders the accounts list page.
func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	accounts, err := h.accountRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching accounts: %v", err)
		http.Error(w, "Error loading accounts", http.StatusInternalServerError)
		return
	}

	categories, err := h.categoryRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching categories: %v", err)
		http.Error(w, "Error loading categories", http.StatusInternalServerError)
		return
	}

	// Create a map for category lookup
	categoryMap := make(map[int64]*models.Category)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	// Build accounts with category info, balance, and holdings
	type AccountWithCategory struct {
		*models.Account
		Category      *models.Category
		Balance       float64
		Holdings      []*models.Holding
		HoldingsValue float64
	}
	accountsWithCat := make([]AccountWithCategory, len(accounts))
	for i, acc := range accounts {
		var cat *models.Category
		if acc.CategoryID != nil {
			cat = categoryMap[*acc.CategoryID]
		}
		balance, _ := h.transactionRepo.GetLatestBalance(acc.ID)

		// Fetch holdings for this account
		holdings, _ := h.holdingRepo.GetByAccountID(acc.ID)
		var holdingsValue float64
		for _, h := range holdings {
			holdingsValue += h.CurrentValue
		}

		accountsWithCat[i] = AccountWithCategory{
			Account:       acc,
			Category:      cat,
			Balance:       balance,
			Holdings:      holdings,
			HoldingsValue: holdingsValue,
		}
	}

	// Count assets and liabilities
	assetCount, _ := h.accountRepo.CountActiveAssets(user.ID)
	liabilityCount, _ := h.accountRepo.CountActiveLiabilities(user.ID)

	h.render(w, "accounts.html", map[string]any{
		"Title":          "Accounts",
		"User":           user,
		"ActiveNav":      "accounts",
		"Accounts":       accountsWithCat,
		"Categories":     categories,
		"AssetCount":     assetCount,
		"LiabilityCount": liabilityCount,
	})
}

// Create handles creating a new account.
func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, user, "Invalid form data")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	categoryIDStr := r.FormValue("category_id")
	notes := strings.TrimSpace(r.FormValue("notes"))
	isLiability := r.FormValue("is_liability") == "1"

	// Validate
	if name == "" {
		h.renderError(w, r, user, "Name is required")
		return
	}

	// Default currency
	if currency == "" {
		currency = "DKK"
	}

	// Parse category ID
	var categoryID *int64
	if categoryIDStr != "" && categoryIDStr != "0" {
		id, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil {
			// Verify category belongs to user
			cat, _ := h.categoryRepo.GetByID(id)
			if cat != nil && cat.UserID == user.ID {
				categoryID = &id
			}
		}
	}

	account := &models.Account{
		UserID:      user.ID,
		CategoryID:  categoryID,
		Name:        name,
		Currency:    currency,
		IsLiability: isLiability,
		IsActive:    true,
		Notes:       notes,
	}

	_, err := h.accountRepo.Create(account)
	if err != nil {
		log.Printf("Error creating account: %v", err)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			h.renderError(w, r, user, "An account with this name already exists")
			return
		}
		h.renderError(w, r, user, "Failed to create account")
		return
	}

	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// Update handles updating an account.
func (h *AccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Check for method override (DELETE)
	if r.FormValue("_method") == "DELETE" {
		h.Delete(w, r)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	// Verify account belongs to user
	existing, err := h.accountRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	categoryIDStr := r.FormValue("category_id")
	notes := strings.TrimSpace(r.FormValue("notes"))
	isLiability := r.FormValue("is_liability") == "1"
	isActive := r.FormValue("is_active") != "0"

	// Validate
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Parse category ID
	var categoryID *int64
	if categoryIDStr != "" && categoryIDStr != "0" {
		catID, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil {
			// Verify category belongs to user
			cat, _ := h.categoryRepo.GetByID(catID)
			if cat != nil && cat.UserID == user.ID {
				categoryID = &catID
			}
		}
	}

	existing.Name = name
	existing.Currency = currency
	existing.CategoryID = categoryID
	existing.Notes = notes
	existing.IsLiability = isLiability
	existing.IsActive = isActive

	err = h.accountRepo.Update(existing)
	if err != nil {
		log.Printf("Error updating account: %v", err)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "An account with this name already exists", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to update account", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// Delete handles deleting an account.
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	// Verify account belongs to user
	existing, err := h.accountRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = h.accountRepo.Delete(id)
	if err != nil {
		log.Printf("Error deleting account: %v", err)
		http.Error(w, "Failed to delete account", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// UpdateBalance handles updating an account's balance by creating a transaction.
func (h *AccountHandler) UpdateBalance(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	// Verify account belongs to user
	account, err := h.accountRepo.GetByID(id)
	if err != nil || account == nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if account.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse new balance
	newBalanceStr := r.FormValue("balance")
	newBalance, err := strconv.ParseFloat(newBalanceStr, 64)
	if err != nil {
		http.Error(w, "Invalid balance value", http.StatusBadRequest)
		return
	}

	// Get current balance
	currentBalance, _ := h.transactionRepo.GetLatestBalance(id)

	// Calculate the difference (transaction amount)
	amount := newBalance - currentBalance

	// Only create transaction if there's a change
	if amount != 0 {
		txn := &models.Transaction{
			AccountID:       id,
			Amount:          amount,
			BalanceAfter:    newBalance,
			Description:     "Balance update",
			TransactionDate: time.Now(),
		}

		_, err = h.transactionRepo.Create(txn)
		if err != nil {
			log.Printf("Error creating balance update transaction: %v", err)
			http.Error(w, "Failed to update balance", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// render renders a template with the given data.
func (h *AccountHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// renderError re-renders the accounts page with an error message.
func (h *AccountHandler) renderError(w http.ResponseWriter, r *http.Request, user *models.User, errMsg string) {
	accounts, _ := h.accountRepo.GetByUserID(user.ID)
	categories, _ := h.categoryRepo.GetByUserID(user.ID)

	categoryMap := make(map[int64]*models.Category)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	type AccountWithCategory struct {
		*models.Account
		Category      *models.Category
		Balance       float64
		Holdings      []*models.Holding
		HoldingsValue float64
	}
	accountsWithCat := make([]AccountWithCategory, len(accounts))
	for i, acc := range accounts {
		var cat *models.Category
		if acc.CategoryID != nil {
			cat = categoryMap[*acc.CategoryID]
		}
		balance, _ := h.transactionRepo.GetLatestBalance(acc.ID)

		holdings, _ := h.holdingRepo.GetByAccountID(acc.ID)
		var holdingsValue float64
		for _, hld := range holdings {
			holdingsValue += hld.CurrentValue
		}

		accountsWithCat[i] = AccountWithCategory{
			Account:       acc,
			Category:      cat,
			Balance:       balance,
			Holdings:      holdings,
			HoldingsValue: holdingsValue,
		}
	}

	assetCount, _ := h.accountRepo.CountActiveAssets(user.ID)
	liabilityCount, _ := h.accountRepo.CountActiveLiabilities(user.ID)

	h.render(w, "accounts.html", map[string]any{
		"Title":          "Accounts",
		"User":           user,
		"ActiveNav":      "accounts",
		"Accounts":       accountsWithCat,
		"Categories":     categories,
		"AssetCount":     assetCount,
		"LiabilityCount": liabilityCount,
		"Error":          errMsg,
	})
}
