package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// CategoryHandler handles category routes.
type CategoryHandler struct {
	templates    map[string]*template.Template
	categoryRepo *repository.CategoryRepository
	accountRepo  *repository.AccountRepository
}

// NewCategoryHandler creates a new CategoryHandler.
func NewCategoryHandler(
	templates map[string]*template.Template,
	categoryRepo *repository.CategoryRepository,
	accountRepo *repository.AccountRepository,
) *CategoryHandler {
	return &CategoryHandler{
		templates:    templates,
		categoryRepo: categoryRepo,
		accountRepo:  accountRepo,
	}
}

// List renders the categories list page.
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	categories, err := h.categoryRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching categories: %v", err)
		http.Error(w, "Error loading categories", http.StatusInternalServerError)
		return
	}

	// Get account counts for each category
	type CategoryWithCount struct {
		*models.Category
		AccountCount int
	}
	categoriesWithCounts := make([]CategoryWithCount, len(categories))
	for i, cat := range categories {
		accounts, _ := h.accountRepo.GetByCategoryID(cat.ID)
		categoriesWithCounts[i] = CategoryWithCount{
			Category:     cat,
			AccountCount: len(accounts),
		}
	}

	h.render(w, "categories.html", map[string]any{
		"Title":      "Categories",
		"User":       user,
		"ActiveNav":  "categories",
		"Categories": categoriesWithCounts,
		"DemoMode":   IsDemoMode(),
	})
}

// Create handles creating a new category.
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
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
	color := strings.TrimSpace(r.FormValue("color"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	sortOrderStr := r.FormValue("sort_order")

	// Validate
	if name == "" {
		h.renderError(w, r, user, "Name is required")
		return
	}

	// Check for duplicate name
	exists, err := h.categoryRepo.NameExists(user.ID, name, 0)
	if err != nil {
		log.Printf("Error checking category name: %v", err)
		h.renderError(w, r, user, "An error occurred")
		return
	}
	if exists {
		h.renderError(w, r, user, "A category with this name already exists")
		return
	}

	// Default color if not provided
	if color == "" {
		color = "#6366f1"
	}

	// Parse sort order
	sortOrder := 0
	if sortOrderStr != "" {
		sortOrder, _ = strconv.Atoi(sortOrderStr)
	}

	category := &models.Category{
		UserID:    user.ID,
		Name:      name,
		Color:     color,
		Icon:      icon,
		SortOrder: sortOrder,
	}

	_, err = h.categoryRepo.Create(category)
	if err != nil {
		log.Printf("Error creating category: %v", err)
		h.renderError(w, r, user, "Failed to create category")
		return
	}

	// Redirect back to categories list
	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

// Update handles updating a category.
func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	// Verify category belongs to user
	existing, err := h.categoryRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	color := strings.TrimSpace(r.FormValue("color"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	sortOrderStr := r.FormValue("sort_order")

	// Validate
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Check for duplicate name (excluding current category)
	exists, err := h.categoryRepo.NameExists(user.ID, name, id)
	if err != nil {
		log.Printf("Error checking category name: %v", err)
		http.Error(w, "An error occurred", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "A category with this name already exists", http.StatusBadRequest)
		return
	}

	// Parse sort order
	sortOrder := existing.SortOrder
	if sortOrderStr != "" {
		sortOrder, _ = strconv.Atoi(sortOrderStr)
	}

	existing.Name = name
	existing.Color = color
	existing.Icon = icon
	existing.SortOrder = sortOrder

	err = h.categoryRepo.Update(existing)
	if err != nil {
		log.Printf("Error updating category: %v", err)
		http.Error(w, "Failed to update category", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

// Delete handles deleting a category.
func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	// Verify category belongs to user
	existing, err := h.categoryRepo.GetByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if existing.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = h.categoryRepo.Delete(id)
	if err != nil {
		log.Printf("Error deleting category: %v", err)
		http.Error(w, "Failed to delete category", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

// render renders a template with the given data.
func (h *CategoryHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// renderError re-renders the categories page with an error message.
func (h *CategoryHandler) renderError(w http.ResponseWriter, r *http.Request, user *models.User, errMsg string) {
	categories, _ := h.categoryRepo.GetByUserID(user.ID)

	type CategoryWithCount struct {
		*models.Category
		AccountCount int
	}
	categoriesWithCounts := make([]CategoryWithCount, len(categories))
	for i, cat := range categories {
		accounts, _ := h.accountRepo.GetByCategoryID(cat.ID)
		categoriesWithCounts[i] = CategoryWithCount{
			Category:     cat,
			AccountCount: len(accounts),
		}
	}

	h.render(w, "categories.html", map[string]any{
		"Title":      "Categories",
		"User":       user,
		"ActiveNav":  "categories",
		"Categories": categoriesWithCounts,
		"Error":      errMsg,
	})
}
