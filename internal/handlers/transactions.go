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

// TransactionHandler handles transaction routes.
type TransactionHandler struct {
	templates       map[string]*template.Template
	transactionRepo *repository.TransactionRepository
	accountRepo     *repository.AccountRepository
	categoryRepo    *repository.CategoryRepository
}

// NewTransactionHandler creates a new TransactionHandler.
func NewTransactionHandler(
	templates map[string]*template.Template,
	transactionRepo *repository.TransactionRepository,
	accountRepo *repository.AccountRepository,
	categoryRepo *repository.CategoryRepository,
) *TransactionHandler {
	return &TransactionHandler{
		templates:       templates,
		transactionRepo: transactionRepo,
		accountRepo:     accountRepo,
		categoryRepo:    categoryRepo,
	}
}

// List renders the transactions list page.
func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Get pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	// Get account filter
	accountIDStr := r.URL.Query().Get("account")
	var accountID int64
	if accountIDStr != "" {
		accountID, _ = strconv.ParseInt(accountIDStr, 10, 64)
	}

	var transactions []*models.Transaction
	var err error

	if accountID > 0 {
		// Verify account belongs to user
		account, _ := h.accountRepo.GetByID(accountID)
		if account != nil && account.UserID == user.ID {
			transactions, err = h.transactionRepo.GetByAccountID(accountID, limit, offset)
		}
	} else {
		transactions, err = h.transactionRepo.GetByUserID(user.ID, limit, offset)
	}

	if err != nil {
		log.Printf("Error fetching transactions: %v", err)
		http.Error(w, "Error loading transactions", http.StatusInternalServerError)
		return
	}

	// Get accounts for filter dropdown and to enrich transactions
	accounts, _ := h.accountRepo.GetByUserID(user.ID)
	accountMap := make(map[int64]*models.Account)
	for _, acc := range accounts {
		accountMap[acc.ID] = acc
	}

	// Build transactions with account info
	type TransactionWithAccount struct {
		*models.Transaction
		Account *models.Account
	}
	txnsWithAccount := make([]TransactionWithAccount, len(transactions))
	for i, txn := range transactions {
		txnsWithAccount[i] = TransactionWithAccount{
			Transaction: txn,
			Account:     accountMap[txn.AccountID],
		}
	}

	h.render(w, "transactions.html", map[string]any{
		"Title":           "Transactions",
		"User":            user,
		"ActiveNav":       "transactions",
		"Transactions":    txnsWithAccount,
		"Accounts":        accounts,
		"SelectedAccount": accountID,
		"Page":            page,
		"HasMore":         len(transactions) == limit,
		"DemoMode":        IsDemoMode(),
	})
}

// Create handles creating a new transaction.
func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	accountIDStr := r.FormValue("account_id")
	amountStr := r.FormValue("amount")
	description := strings.TrimSpace(r.FormValue("description"))
	dateStr := r.FormValue("transaction_date")

	// Validate account
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid account", http.StatusBadRequest)
		return
	}

	account, err := h.accountRepo.GetByID(accountID)
	if err != nil || account == nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if account.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	// Parse date
	transactionDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		transactionDate = time.Now()
	}

	// Get current balance and calculate new balance
	currentBalance, _ := h.transactionRepo.GetLatestBalance(accountID)

	// For liabilities, amounts work inversely
	newBalance := currentBalance + amount
	if account.IsLiability {
		// For liabilities: positive amount means debt increased, negative means paid down
		newBalance = currentBalance + amount
	}

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          amount,
		BalanceAfter:    newBalance,
		Description:     description,
		TransactionDate: transactionDate,
	}

	_, err = h.transactionRepo.Create(txn)
	if err != nil {
		log.Printf("Error creating transaction: %v", err)
		http.Error(w, "Failed to create transaction", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// Update handles updating a transaction.
func (h *TransactionHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Invalid transaction ID", http.StatusBadRequest)
		return
	}

	// Get existing transaction
	existing, err := h.transactionRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Transaction not found", http.StatusNotFound)
		return
	}

	// Verify account belongs to user
	account, _ := h.accountRepo.GetByID(existing.AccountID)
	if account == nil || account.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	amountStr := r.FormValue("amount")
	description := strings.TrimSpace(r.FormValue("description"))
	dateStr := r.FormValue("transaction_date")

	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	// Parse date
	transactionDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		transactionDate = existing.TransactionDate
	}

	// Recalculate balance (simplified - in production you'd recalculate all subsequent balances)
	balanceDiff := amount - existing.Amount
	newBalance := existing.BalanceAfter + balanceDiff

	existing.Amount = amount
	existing.BalanceAfter = newBalance
	existing.Description = description
	existing.TransactionDate = transactionDate

	err = h.transactionRepo.Update(existing)
	if err != nil {
		log.Printf("Error updating transaction: %v", err)
		http.Error(w, "Failed to update transaction", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// Delete handles deleting a transaction.
func (h *TransactionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid transaction ID", http.StatusBadRequest)
		return
	}

	// Get existing transaction
	existing, err := h.transactionRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Transaction not found", http.StatusNotFound)
		return
	}

	// Verify account belongs to user
	account, _ := h.accountRepo.GetByID(existing.AccountID)
	if account == nil || account.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = h.transactionRepo.Delete(id)
	if err != nil {
		log.Printf("Error deleting transaction: %v", err)
		http.Error(w, "Failed to delete transaction", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// render renders a template with the given data.
func (h *TransactionHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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
