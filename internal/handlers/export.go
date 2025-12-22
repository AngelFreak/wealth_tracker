package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/repository"
)

// ExportHandler handles data export requests.
type ExportHandler struct {
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	categoryRepo    *repository.CategoryRepository
	goalRepo        *repository.GoalRepository
}

// NewExportHandler creates a new export handler.
func NewExportHandler(
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
	categoryRepo *repository.CategoryRepository,
	goalRepo *repository.GoalRepository,
) *ExportHandler {
	return &ExportHandler{
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		goalRepo:        goalRepo,
	}
}

// ExportTransactions exports all transactions as CSV.
func (h *ExportHandler) ExportTransactions(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get all accounts for this user
	accounts, err := h.accountRepo.GetByUserID(user.ID)
	if err != nil {
		http.Error(w, "Failed to get accounts", http.StatusInternalServerError)
		return
	}

	// Build account ID to name map
	accountNames := make(map[int64]string)
	accountCurrencies := make(map[int64]string)
	for _, acc := range accounts {
		accountNames[acc.ID] = acc.Name
		accountCurrencies[acc.ID] = acc.Currency
	}

	// Collect all transactions
	type exportRow struct {
		Date        string
		Account     string
		Currency    string
		Amount      float64
		Balance     float64
		Description string
	}

	var rows []exportRow

	for _, acc := range accounts {
		txs, err := h.transactionRepo.GetByAccountID(acc.ID, 10000, 0) // Get all
		if err != nil {
			continue
		}
		for _, tx := range txs {
			rows = append(rows, exportRow{
				Date:        tx.TransactionDate.Format("2006-01-02"),
				Account:     accountNames[tx.AccountID],
				Currency:    accountCurrencies[tx.AccountID],
				Amount:      tx.Amount,
				Balance:     tx.BalanceAfter,
				Description: tx.Description,
			})
		}
	}

	// Set headers for CSV download
	filename := fmt.Sprintf("transactions_%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Write CSV
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	writer.Write([]string{"Date", "Account", "Currency", "Amount", "Balance After", "Description"})

	// Data rows
	for _, row := range rows {
		writer.Write([]string{
			row.Date,
			row.Account,
			row.Currency,
			strconv.FormatFloat(row.Amount, 'f', 2, 64),
			strconv.FormatFloat(row.Balance, 'f', 2, 64),
			row.Description,
		})
	}
}

// ExportAccounts exports all accounts as CSV.
func (h *ExportHandler) ExportAccounts(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	accounts, err := h.accountRepo.GetByUserID(user.ID)
	if err != nil {
		http.Error(w, "Failed to get accounts", http.StatusInternalServerError)
		return
	}

	// Get categories for names
	categories, _ := h.categoryRepo.GetByUserID(user.ID)
	categoryNames := make(map[int64]string)
	for _, cat := range categories {
		categoryNames[cat.ID] = cat.Name
	}

	// Set headers for CSV download
	filename := fmt.Sprintf("accounts_%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Write CSV
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	writer.Write([]string{"Name", "Category", "Currency", "Balance", "Type", "Status", "Notes"})

	// Data rows
	for _, acc := range accounts {
		accType := "Asset"
		if acc.IsLiability {
			accType = "Liability"
		}
		status := "Active"
		if !acc.IsActive {
			status = "Inactive"
		}
		categoryName := ""
		if acc.CategoryID != nil {
			categoryName = categoryNames[*acc.CategoryID]
		}

		writer.Write([]string{
			acc.Name,
			categoryName,
			acc.Currency,
			strconv.FormatFloat(acc.Balance, 'f', 2, 64),
			accType,
			status,
			acc.Notes,
		})
	}
}

// ExportAll exports all user data as JSON.
func (h *ExportHandler) ExportAll(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Collect all data
	accounts, _ := h.accountRepo.GetByUserID(user.ID)
	categories, _ := h.categoryRepo.GetByUserID(user.ID)
	goals, _ := h.goalRepo.GetByUserID(user.ID)

	// Collect all transactions
	var allTransactions []map[string]interface{}
	for _, acc := range accounts {
		txs, err := h.transactionRepo.GetByAccountID(acc.ID, 10000, 0)
		if err != nil {
			continue
		}
		for _, tx := range txs {
			allTransactions = append(allTransactions, map[string]interface{}{
				"account_id":       tx.AccountID,
				"amount":           tx.Amount,
				"balance_after":    tx.BalanceAfter,
				"description":      tx.Description,
				"transaction_date": tx.TransactionDate.Format("2006-01-02"),
			})
		}
	}

	// Build export structure
	export := map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"user": map[string]interface{}{
			"name":             user.Name,
			"email":            user.Email,
			"default_currency": user.DefaultCurrency,
		},
		"categories":   categories,
		"accounts":     accounts,
		"transactions": allTransactions,
		"goals":        goals,
	}

	// Set headers for JSON download
	filename := fmt.Sprintf("wealth_tracker_backup_%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Write JSON with pretty formatting
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(export)
}
