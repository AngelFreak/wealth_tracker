package handlers

import (
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// GoalHandler handles goal routes.
type GoalHandler struct {
	templates       map[string]*template.Template
	goalRepo        *repository.GoalRepository
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	categoryRepo    *repository.CategoryRepository
}

// NewGoalHandler creates a new GoalHandler.
func NewGoalHandler(
	templates map[string]*template.Template,
	goalRepo *repository.GoalRepository,
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
	categoryRepo *repository.CategoryRepository,
) *GoalHandler {
	return &GoalHandler{
		templates:       templates,
		goalRepo:        goalRepo,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
	}
}

// List renders the goals list page.
func (h *GoalHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	goals, err := h.goalRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching goals: %v", err)
		http.Error(w, "Error loading goals", http.StatusInternalServerError)
		return
	}

	// Fetch categories for dropdown and display
	categories, err := h.categoryRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching categories: %v", err)
		categories = []*models.Category{}
	}

	// Build category map for quick lookup
	categoryMap := make(map[int64]*models.Category)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	// Calculate current net worth for progress calculation
	netWorth := h.calculateNetWorth(user.ID)

	// Calculate progress for each goal
	type GoalWithProgress struct {
		*models.Goal
		Progress     float64
		IsReached    bool
		DaysLeft     *int
		IsOverdue    bool
		CurrentWorth float64
		Category     *models.Category
	}

	goalsWithProgress := make([]GoalWithProgress, len(goals))
	for i, goal := range goals {
		// Determine current worth based on whether goal is category-specific
		currentWorth := netWorth
		if goal.CategoryID != nil {
			currentWorth = h.calculateCategoryNetWorth(user.ID, *goal.CategoryID)
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

		// Auto-mark goal as reached in database if not already
		if isReached && goal.ReachedDate == nil {
			_ = h.goalRepo.MarkAsReached(goal.ID)
		}

		gwp := GoalWithProgress{
			Goal:         goal,
			Progress:     progress,
			IsReached:    isReached,
			CurrentWorth: currentWorth,
		}

		// Add category info if goal has a category
		if goal.CategoryID != nil {
			gwp.Category = categoryMap[*goal.CategoryID]
		}

		// Calculate days left if deadline is set and goal not reached
		if goal.Deadline != nil && !isReached {
			now := time.Now()
			daysLeft := int(goal.Deadline.Sub(now).Hours() / 24)
			gwp.DaysLeft = &daysLeft
			gwp.IsOverdue = daysLeft < 0
		}

		goalsWithProgress[i] = gwp
	}

	// Count stats
	totalGoals, _ := h.goalRepo.CountByUserID(user.ID)
	reachedGoals, _ := h.goalRepo.CountReachedByUserID(user.ID)

	h.render(w, "goals.html", map[string]any{
		"Title":        "Goals",
		"User":         user,
		"ActiveNav":    "goals",
		"Goals":        goalsWithProgress,
		"TotalGoals":   totalGoals,
		"ReachedGoals": reachedGoals,
		"NetWorth":     netWorth,
		"Categories":   categories,
	})
}

// Create handles creating a new goal.
func (h *GoalHandler) Create(w http.ResponseWriter, r *http.Request) {
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
	targetAmountStr := r.FormValue("target_amount")
	targetCurrency := strings.TrimSpace(r.FormValue("target_currency"))
	deadlineStr := r.FormValue("deadline")
	categoryIDStr := r.FormValue("category_id")

	// Validate
	if name == "" {
		h.renderError(w, r, user, "Name is required")
		return
	}

	targetAmount, err := strconv.ParseFloat(targetAmountStr, 64)
	if err != nil || targetAmount <= 0 {
		h.renderError(w, r, user, "Target amount must be a positive number")
		return
	}

	// Default currency
	if targetCurrency == "" {
		targetCurrency = user.DefaultCurrency
	}

	// Parse deadline
	var deadline *time.Time
	if deadlineStr != "" {
		d, err := time.Parse("2006-01-02", deadlineStr)
		if err == nil {
			deadline = &d
		}
	}

	// Parse category ID (optional)
	var categoryID *int64
	if categoryIDStr != "" {
		cid, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil && cid > 0 {
			categoryID = &cid
		}
	}

	goal := &models.Goal{
		UserID:         user.ID,
		CategoryID:     categoryID,
		Name:           name,
		TargetAmount:   targetAmount,
		TargetCurrency: targetCurrency,
		Deadline:       deadline,
	}

	_, err = h.goalRepo.Create(goal)
	if err != nil {
		log.Printf("Error creating goal: %v", err)
		h.renderError(w, r, user, "Failed to create goal")
		return
	}

	http.Redirect(w, r, "/goals", http.StatusSeeOther)
}

// Update handles updating a goal.
func (h *GoalHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	// Verify goal belongs to user
	existing, err := h.goalRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Goal not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	targetAmountStr := r.FormValue("target_amount")
	targetCurrency := strings.TrimSpace(r.FormValue("target_currency"))
	deadlineStr := r.FormValue("deadline")
	reachedDateStr := r.FormValue("reached_date")
	categoryIDStr := r.FormValue("category_id")

	// Validate
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	targetAmount, err := strconv.ParseFloat(targetAmountStr, 64)
	if err != nil || targetAmount <= 0 {
		http.Error(w, "Target amount must be a positive number", http.StatusBadRequest)
		return
	}

	// Parse deadline
	var deadline *time.Time
	if deadlineStr != "" {
		d, err := time.Parse("2006-01-02", deadlineStr)
		if err == nil {
			deadline = &d
		}
	}

	// Parse reached date
	var reachedDate *time.Time
	if reachedDateStr != "" {
		rd, err := time.Parse("2006-01-02", reachedDateStr)
		if err == nil {
			reachedDate = &rd
		}
	}

	// Parse category ID (optional - empty string means global/all assets)
	var categoryID *int64
	if categoryIDStr != "" {
		cid, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil && cid > 0 {
			categoryID = &cid
		}
	}

	existing.Name = name
	existing.TargetAmount = targetAmount
	existing.TargetCurrency = targetCurrency
	existing.Deadline = deadline
	existing.ReachedDate = reachedDate
	existing.CategoryID = categoryID

	err = h.goalRepo.Update(existing)
	if err != nil {
		log.Printf("Error updating goal: %v", err)
		http.Error(w, "Failed to update goal", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/goals", http.StatusSeeOther)
}

// Delete handles deleting a goal.
func (h *GoalHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	// Verify goal belongs to user
	existing, err := h.goalRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Goal not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = h.goalRepo.Delete(id)
	if err != nil {
		log.Printf("Error deleting goal: %v", err)
		http.Error(w, "Failed to delete goal", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/goals", http.StatusSeeOther)
}

// calculateNetWorth calculates the user's current net worth.
func (h *GoalHandler) calculateNetWorth(userID int64) float64 {
	accounts, err := h.accountRepo.GetByUserIDActiveOnly(userID)
	if err != nil {
		return 0
	}

	netWorth := 0.0
	for _, acc := range accounts {
		// Get balance from transactions
		balance, err := h.transactionRepo.GetLatestBalance(acc.ID)
		if err != nil {
			continue
		}
		if acc.IsLiability {
			// Use absolute value to handle both positive and negative storage
			netWorth -= math.Abs(balance)
		} else {
			netWorth += balance
		}
	}
	return netWorth
}

// calculateCategoryNetWorth calculates the net worth for a specific category.
func (h *GoalHandler) calculateCategoryNetWorth(userID, categoryID int64) float64 {
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

		// Get balance from transactions
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

// render renders a template with the given data.
func (h *GoalHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// renderError re-renders the goals page with an error message.
func (h *GoalHandler) renderError(w http.ResponseWriter, r *http.Request, user *models.User, errMsg string) {
	goals, _ := h.goalRepo.GetByUserID(user.ID)
	netWorth := h.calculateNetWorth(user.ID)

	// Fetch categories for dropdown and display
	categories, _ := h.categoryRepo.GetByUserID(user.ID)
	categoryMap := make(map[int64]*models.Category)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	type GoalWithProgress struct {
		*models.Goal
		Progress     float64
		IsReached    bool
		DaysLeft     *int
		IsOverdue    bool
		CurrentWorth float64
		Category     *models.Category
	}

	goalsWithProgress := make([]GoalWithProgress, len(goals))
	for i, goal := range goals {
		// Determine current worth based on whether goal is category-specific
		currentWorth := netWorth
		if goal.CategoryID != nil {
			currentWorth = h.calculateCategoryNetWorth(user.ID, *goal.CategoryID)
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
			Goal:         goal,
			Progress:     progress,
			IsReached:    isReached,
			CurrentWorth: currentWorth,
		}

		// Add category info if goal has a category
		if goal.CategoryID != nil {
			gwp.Category = categoryMap[*goal.CategoryID]
		}

		// Calculate days left if deadline is set and goal not reached
		if goal.Deadline != nil && !isReached {
			now := time.Now()
			daysLeft := int(goal.Deadline.Sub(now).Hours() / 24)
			gwp.DaysLeft = &daysLeft
			gwp.IsOverdue = daysLeft < 0
		}

		goalsWithProgress[i] = gwp
	}

	totalGoals, _ := h.goalRepo.CountByUserID(user.ID)
	reachedGoals, _ := h.goalRepo.CountReachedByUserID(user.ID)

	h.render(w, "goals.html", map[string]any{
		"Title":        "Goals",
		"User":         user,
		"ActiveNav":    "goals",
		"Goals":        goalsWithProgress,
		"TotalGoals":   totalGoals,
		"ReachedGoals": reachedGoals,
		"NetWorth":     netWorth,
		"Categories":   categories,
		"Error":        errMsg,
	})
}
