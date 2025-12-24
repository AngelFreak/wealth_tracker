package handlers

import (
	"html/template"
	"log"
	"math"
	"net/http"
	"time"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// DashboardHandler handles dashboard routes.
type DashboardHandler struct {
	templates       map[string]*template.Template
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	goalRepo        *repository.GoalRepository
	categoryRepo    *repository.CategoryRepository
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(
	templates map[string]*template.Template,
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
	goalRepo *repository.GoalRepository,
	categoryRepo *repository.CategoryRepository,
) *DashboardHandler {
	return &DashboardHandler{
		templates:       templates,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		goalRepo:        goalRepo,
		categoryRepo:    categoryRepo,
	}
}

// Dashboard renders the main dashboard page.
func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Calculate financial statistics
	netWorth, totalAssets, totalLiabilities, assetCount, liabilityCount := h.calculateStats(user.ID)

	// Calculate monthly change
	monthlyChange, monthlyPercent := h.calculateMonthlyChange(user.ID, netWorth)

	// Get recent transactions (limit 5)
	recentTransactions, _ := h.transactionRepo.GetRecentByUserID(user.ID, 5)

	// Get goals with progress
	goals, _ := h.goalRepo.GetByUserID(user.ID)
	goalsWithProgress := h.calculateGoalProgress(user.ID, goals, netWorth)

	// Get categories with totals for asset distribution
	categories, _ := h.categoryRepo.GetByUserID(user.ID)
	categoryTotals := h.calculateCategoryTotals(user.ID, categories)

	// Get net worth history for chart
	netWorthHistory, _ := h.transactionRepo.GetNetWorthHistory(user.ID)

	// Check if admin is impersonating
	_, impersonating := r.Cookie("admin_session_id")

	h.render(w, "dashboard.html", map[string]any{
		"Title":              "Dashboard",
		"User":               user,
		"ActiveNav":          "dashboard",
		"NetWorth":           netWorth,
		"TotalAssets":        totalAssets,
		"TotalLiabilities":   totalLiabilities,
		"AssetCount":         assetCount,
		"LiabilityCount":     liabilityCount,
		"MonthlyChange":      monthlyChange,
		"MonthlyPercent":     monthlyPercent,
		"RecentTransactions": recentTransactions,
		"Goals":              goalsWithProgress,
		"CategoryTotals":     categoryTotals,
		"NetWorthHistory":    netWorthHistory,
		"IncludeCharts":      true,
		"Impersonating":      impersonating == nil,
		"DemoMode":           IsDemoMode(),
	})
}

// calculateStats calculates net worth, assets, liabilities, and counts.
func (h *DashboardHandler) calculateStats(userID int64) (netWorth, totalAssets, totalLiabilities float64, assetCount, liabilityCount int) {
	accounts, err := h.accountRepo.GetByUserIDActiveOnly(userID)
	if err != nil {
		return 0, 0, 0, 0, 0
	}

	for _, acc := range accounts {
		balance, err := h.transactionRepo.GetLatestBalance(acc.ID)
		if err != nil {
			continue
		}
		if acc.IsLiability {
			// Use absolute value to handle both positive and negative storage
			totalLiabilities += math.Abs(balance)
			liabilityCount++
		} else {
			totalAssets += balance
			assetCount++
		}
	}
	netWorth = totalAssets - totalLiabilities
	return
}

// calculateMonthlyChange calculates the change in net worth this month.
func (h *DashboardHandler) calculateMonthlyChange(userID int64, currentNetWorth float64) (change float64, percent float64) {
	// Get start of current month
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Sum all transactions this month
	accounts, _ := h.accountRepo.GetByUserIDActiveOnly(userID)
	for _, acc := range accounts {
		monthlySum, _ := h.transactionRepo.GetSumSince(acc.ID, startOfMonth)
		if acc.IsLiability {
			// Use absolute value to handle both positive and negative storage
			change -= math.Abs(monthlySum)
		} else {
			change += monthlySum
		}
	}

	// Calculate percentage
	if currentNetWorth != 0 && currentNetWorth != change {
		previousWorth := currentNetWorth - change
		if previousWorth != 0 {
			percent = (change / previousWorth) * 100
		}
	}
	return
}

// GoalWithProgress represents a goal with its progress info.
type GoalWithProgress struct {
	*models.Goal
	Progress   float64
	IsReached  bool
	DaysLeft   *int
	IsOverdue  bool
}

// calculateGoalProgress calculates progress for each goal.
func (h *DashboardHandler) calculateGoalProgress(userID int64, goals []*models.Goal, netWorth float64) []GoalWithProgress {
	result := make([]GoalWithProgress, 0, len(goals))
	for _, goal := range goals {
		// Determine current worth based on whether goal is category-specific
		currentWorth := netWorth
		if goal.CategoryID != nil {
			currentWorth = h.calculateCategoryNetWorth(userID, *goal.CategoryID)
		}

		progress := 0.0
		if goal.TargetAmount > 0 {
			progress = (currentWorth / goal.TargetAmount) * 100
			if progress > 100 {
				progress = 100
			}
		}

		// Goal is reached if progress >= 100% OR already marked in database
		isReached := goal.ReachedDate != nil || currentWorth >= goal.TargetAmount

		gwp := GoalWithProgress{
			Goal:      goal,
			Progress:  progress,
			IsReached: isReached,
		}

		// Calculate days left if deadline is set and goal not reached
		if goal.Deadline != nil && !isReached {
			now := time.Now()
			daysLeft := int(goal.Deadline.Sub(now).Hours() / 24)
			gwp.DaysLeft = &daysLeft
			gwp.IsOverdue = daysLeft < 0
		}

		result = append(result, gwp)
	}
	return result
}

// calculateCategoryNetWorth calculates the net worth for a specific category.
func (h *DashboardHandler) calculateCategoryNetWorth(userID, categoryID int64) float64 {
	accounts, err := h.accountRepo.GetByUserIDActiveOnly(userID)
	if err != nil {
		return 0
	}

	netWorth := 0.0
	for _, acc := range accounts {
		// Skip accounts not in this category
		if acc.CategoryID == nil || *acc.CategoryID != categoryID {
			continue
		}

		balance, err := h.transactionRepo.GetLatestBalance(acc.ID)
		if err != nil {
			continue
		}
		if acc.IsLiability {
			netWorth -= math.Abs(balance)
		} else {
			netWorth += balance
		}
	}
	return netWorth
}

// CategoryTotal represents a category with its total value.
type CategoryTotal struct {
	*models.Category
	Total float64
}

// calculateCategoryTotals calculates total value for each category.
func (h *DashboardHandler) calculateCategoryTotals(userID int64, categories []*models.Category) []CategoryTotal {
	result := make([]CategoryTotal, 0, len(categories))
	for _, cat := range categories {
		accounts, _ := h.accountRepo.GetByCategoryID(cat.ID)
		total := 0.0
		for _, acc := range accounts {
			if acc.UserID == userID && !acc.IsLiability {
				balance, _ := h.transactionRepo.GetLatestBalance(acc.ID)
				total += balance
			}
		}
		if total > 0 {
			result = append(result, CategoryTotal{Category: cat, Total: total})
		}
	}
	return result
}

// render renders a template with the given data.
func (h *DashboardHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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
