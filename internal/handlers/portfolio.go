package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
	"wealth_tracker/internal/services"
)

// PortfolioHandler handles portfolio analysis and optimization routes.
type PortfolioHandler struct {
	templates        map[string]*template.Template
	portfolioService *services.PortfolioService
	targetRepo       *repository.AllocationTargetRepository
	categoryRepo     *repository.CategoryRepository
}

// NewPortfolioHandler creates a new PortfolioHandler.
func NewPortfolioHandler(
	templates map[string]*template.Template,
	portfolioService *services.PortfolioService,
	targetRepo *repository.AllocationTargetRepository,
	categoryRepo *repository.CategoryRepository,
) *PortfolioHandler {
	return &PortfolioHandler{
		templates:        templates,
		portfolioService: portfolioService,
		targetRepo:       targetRepo,
		categoryRepo:     categoryRepo,
	}
}

// Analyzer renders the portfolio analyzer page.
func (h *PortfolioHandler) Analyzer(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Get portfolio composition
	composition, err := h.portfolioService.GetPortfolioComposition(user.ID)
	if err != nil {
		log.Printf("Error getting portfolio composition: %v", err)
		composition = &services.PortfolioComposition{}
	}

	// Get categories for target dropdown
	categories, err := h.categoryRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error getting categories: %v", err)
		categories = []*models.Category{}
	}

	// Get all allocation targets
	targets, err := h.targetRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error getting allocation targets: %v", err)
		targets = []*models.AllocationTarget{}
	}

	// Convert to JSON for Alpine.js
	compositionJSON, _ := json.Marshal(composition)
	categoriesJSON, _ := json.Marshal(categories)
	targetsJSON, _ := json.Marshal(targets)

	h.render(w, "portfolio-analyzer.html", map[string]any{
		"Title":           "Portfolio Analyzer",
		"User":            user,
		"ActiveNav":       "tools",
		"Composition":     composition,
		"CompositionJSON": template.JS(compositionJSON),
		"Categories":      categories,
		"CategoriesJSON":  template.JS(categoriesJSON),
		"Targets":         targets,
		"TargetsJSON":     template.JS(targetsJSON),
		"DemoMode":        IsDemoMode(),
	})
}

// GetComposition returns the portfolio composition as JSON.
func (h *PortfolioHandler) GetComposition(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	composition, err := h.portfolioService.GetPortfolioComposition(user.ID)
	if err != nil {
		http.Error(w, "Failed to get portfolio composition", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(composition); err != nil {
		log.Printf("Error encoding portfolio composition: %v", err)
	}
}

// GetTargets returns all allocation targets for the user.
func (h *PortfolioHandler) GetTargets(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targets, err := h.targetRepo.GetByUserID(user.ID)
	if err != nil {
		http.Error(w, "Failed to get allocation targets", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(targets); err != nil {
		log.Printf("Error encoding allocation targets: %v", err)
	}
}

// SaveTarget creates or updates an allocation target.
func (h *PortfolioHandler) SaveTarget(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var target models.AllocationTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate target type
	switch target.TargetType {
	case models.TargetTypeCategory, models.TargetTypeAssetType, models.TargetTypeCurrency:
		// Valid
	default:
		http.Error(w, "Invalid target_type", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if target.TargetKey == "" {
		http.Error(w, "target_key is required", http.StatusBadRequest)
		return
	}
	if target.TargetPct < 0 || target.TargetPct > 100 {
		http.Error(w, "target_pct must be between 0 and 100", http.StatusBadRequest)
		return
	}

	target.UserID = user.ID

	// Upsert the target
	id, err := h.targetRepo.Upsert(&target)
	if err != nil {
		log.Printf("Error saving allocation target: %v", err)
		http.Error(w, "Failed to save allocation target", http.StatusInternalServerError)
		return
	}

	target.ID = id

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(target); err != nil {
		log.Printf("Error encoding saved target: %v", err)
	}
}

// DeleteTarget removes an allocation target.
func (h *PortfolioHandler) DeleteTarget(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get target ID from query param or body
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id parameter required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	// Verify ownership (consistent error to prevent enumeration)
	target, err := h.targetRepo.GetByID(id)
	if err != nil || target == nil || target.UserID != user.ID {
		http.Error(w, "Target not found", http.StatusNotFound)
		if err != nil {
			log.Printf("Error getting target %d: %v", id, err)
		}
		return
	}

	if err := h.targetRepo.Delete(id); err != nil {
		http.Error(w, "Failed to delete target", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding delete response: %v", err)
	}
}

// GetComparison returns allocation comparison as JSON.
func (h *PortfolioHandler) GetComparison(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetType := r.URL.Query().Get("type")
	if targetType == "" {
		targetType = models.TargetTypeCategory
	}

	// Validate target type
	switch targetType {
	case models.TargetTypeCategory, models.TargetTypeAssetType, models.TargetTypeCurrency:
		// Valid
	default:
		http.Error(w, "Invalid target type", http.StatusBadRequest)
		return
	}

	comparison, err := h.portfolioService.GetAllocationComparison(user.ID, targetType)
	if err != nil {
		http.Error(w, "Failed to get allocation comparison", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(comparison); err != nil {
		log.Printf("Error encoding comparison: %v", err)
	}
}

// GetRebalancing calculates rebalancing recommendations.
func (h *PortfolioHandler) GetRebalancing(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetType := r.URL.Query().Get("type")
	if targetType == "" {
		targetType = models.TargetTypeCategory
	}

	// Validate target type
	switch targetType {
	case models.TargetTypeCategory, models.TargetTypeAssetType, models.TargetTypeCurrency:
		// Valid
	default:
		http.Error(w, "Invalid target type", http.StatusBadRequest)
		return
	}

	newMoneyStr := r.URL.Query().Get("new_money")
	newMoney := 0.0
	if newMoneyStr != "" {
		var err error
		newMoney, err = strconv.ParseFloat(newMoneyStr, 64)
		if err != nil {
			http.Error(w, "Invalid new_money value", http.StatusBadRequest)
			return
		}
	}

	recommendation, err := h.portfolioService.CalculateRebalancing(user.ID, targetType, newMoney)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to calculate rebalancing: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(recommendation); err != nil {
		log.Printf("Error encoding rebalancing: %v", err)
	}
}

// render renders a template with the given data.
func (h *PortfolioHandler) render(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Buffer template execution to avoid partial writes on error
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("Error rendering template %s: %v", name, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
