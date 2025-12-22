// Package handlers provides HTTP handlers for the wealth tracker.
package handlers

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/database"
	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// ImpersonationCookieName is the cookie name for storing the original admin session.
const ImpersonationCookieName = "admin_session_id"

// AdminHandler handles admin routes.
type AdminHandler struct {
	templates       map[string]*template.Template
	db              *database.DB
	userRepo        *repository.UserRepository
	accountRepo     *repository.AccountRepository
	categoryRepo    *repository.CategoryRepository
	goalRepo        *repository.GoalRepository
	transactionRepo *repository.TransactionRepository
	sessionManager  *auth.SessionManager
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(
	templates map[string]*template.Template,
	db *database.DB,
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
	categoryRepo *repository.CategoryRepository,
	goalRepo *repository.GoalRepository,
	transactionRepo *repository.TransactionRepository,
	sessionManager *auth.SessionManager,
) *AdminHandler {
	return &AdminHandler{
		templates:       templates,
		db:              db,
		userRepo:        userRepo,
		accountRepo:     accountRepo,
		categoryRepo:    categoryRepo,
		goalRepo:        goalRepo,
		transactionRepo: transactionRepo,
		sessionManager:  sessionManager,
	}
}

// Dashboard renders the admin dashboard.
func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Get stats
	userCount, _ := h.userRepo.CountAll()

	// Get table counts
	tableCounts := h.getTableCounts()

	h.render(w, "admin-dashboard.html", map[string]any{
		"Title":       "Admin Dashboard",
		"User":        user,
		"ActiveNav":   "admin",
		"UserCount":   userCount,
		"TableCounts": tableCounts,
		"Impersonating": h.isImpersonating(r),
	})
}

// UserList renders the user list page.
func (h *AdminHandler) UserList(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	users, err := h.userRepo.GetAll()
	if err != nil {
		log.Printf("AdminHandler.UserList error: %v", err)
		http.Error(w, "Error loading users", http.StatusInternalServerError)
		return
	}

	// Enrich users with stats
	type UserWithStats struct {
		*models.User
		AccountCount  int
		CategoryCount int
		GoalCount     int
	}

	var usersWithStats []UserWithStats
	for _, u := range users {
		accountCount, _ := h.accountRepo.CountByUserID(u.ID)
		categoryCount, _ := h.categoryRepo.CountByUserID(u.ID)
		goalCount, _ := h.goalRepo.CountByUserID(u.ID)

		usersWithStats = append(usersWithStats, UserWithStats{
			User:          u,
			AccountCount:  accountCount,
			CategoryCount: categoryCount,
			GoalCount:     goalCount,
		})
	}

	h.render(w, "admin-users.html", map[string]any{
		"Title":         "User Management",
		"User":          user,
		"ActiveNav":     "admin",
		"Users":         usersWithStats,
		"Impersonating": h.isImpersonating(r),
	})
}

// UserView renders the user detail page.
func (h *AdminHandler) UserView(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetUser, err := h.userRepo.GetByID(id)
	if err != nil {
		log.Printf("AdminHandler.UserView error: %v", err)
		http.Error(w, "Error loading user", http.StatusInternalServerError)
		return
	}
	if targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Get user stats
	accountCount, _ := h.accountRepo.CountByUserID(id)
	categoryCount, _ := h.categoryRepo.CountByUserID(id)
	goalCount, _ := h.goalRepo.CountByUserID(id)

	h.render(w, "admin-user-detail.html", map[string]any{
		"Title":         "User Details",
		"User":          user,
		"ActiveNav":     "admin",
		"TargetUser":    targetUser,
		"AccountCount":  accountCount,
		"CategoryCount": categoryCount,
		"GoalCount":     goalCount,
		"Impersonating": h.isImpersonating(r),
	})
}

// UserEdit handles the user edit form submission.
func (h *AdminHandler) UserEdit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetUser, err := h.userRepo.GetByID(id)
	if err != nil || targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	isAdmin := r.FormValue("is_admin") == "1"

	if name == "" || email == "" {
		http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?error=name_email_required", id), http.StatusSeeOther)
		return
	}

	// Update email and name
	if err := h.userRepo.UpdateEmailAndName(id, email, name); err != nil {
		log.Printf("AdminHandler.UserEdit error updating: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?error=update_failed", id), http.StatusSeeOther)
		return
	}

	// Update admin status (don't allow removing own admin status)
	if id != user.ID {
		if err := h.userRepo.SetAdmin(id, isAdmin); err != nil {
			log.Printf("AdminHandler.UserEdit error setting admin: %v", err)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?success=updated", id), http.StatusSeeOther)
}

// UserResetPassword handles password reset for a user.
func (h *AdminHandler) UserResetPassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	newPassword := r.FormValue("new_password")
	if len(newPassword) < 8 {
		http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?error=password_too_short", id), http.StatusSeeOther)
		return
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		log.Printf("AdminHandler.UserResetPassword error hashing: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?error=hash_failed", id), http.StatusSeeOther)
		return
	}

	if err := h.userRepo.UpdatePassword(id, passwordHash); err != nil {
		log.Printf("AdminHandler.UserResetPassword error updating: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?error=update_failed", id), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/users/%d?success=password_reset", id), http.StatusSeeOther)
}

// UserDelete handles user deletion.
func (h *AdminHandler) UserDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Don't allow deleting yourself
	if id == user.ID {
		http.Redirect(w, r, "/admin/users?error=cannot_delete_self", http.StatusSeeOther)
		return
	}

	if err := h.userRepo.Delete(id); err != nil {
		log.Printf("AdminHandler.UserDelete error: %v", err)
		http.Redirect(w, r, "/admin/users?error=delete_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/users?success=deleted", http.StatusSeeOther)
}

// UserImpersonate starts impersonation of another user.
func (h *AdminHandler) UserImpersonate(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Don't allow impersonating yourself
	if id == user.ID {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// Store current admin session
	currentCookie, err := r.Cookie(middleware.SessionCookieName)
	if err != nil {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// Set admin session cookie for later return
	http.SetCookie(w, &http.Cookie{
		Name:     ImpersonationCookieName,
		Value:    currentCookie.Value,
		Path:     "/",
		MaxAge:   3600, // 1 hour
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Create new session for target user
	session, err := h.sessionManager.Create(id)
	if err != nil {
		log.Printf("AdminHandler.UserImpersonate error creating session: %v", err)
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// Set new session cookie
	middleware.SetSessionCookie(w, session.ID, 3600) // 1 hour for impersonation

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ReturnFromImpersonation ends impersonation and returns to admin.
func (h *AdminHandler) ReturnFromImpersonation(w http.ResponseWriter, r *http.Request) {
	// Get admin session cookie
	adminCookie, err := r.Cookie(ImpersonationCookieName)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Delete current impersonation session
	currentCookie, _ := r.Cookie(middleware.SessionCookieName)
	if currentCookie != nil {
		h.sessionManager.Delete(currentCookie.Value)
	}

	// Restore admin session
	middleware.SetSessionCookie(w, adminCookie.Value, 7*24*60*60)

	// Clear impersonation cookie
	http.SetCookie(w, &http.Cookie{
		Name:   ImpersonationCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// DatabaseOverview renders the database overview page.
func (h *AdminHandler) DatabaseOverview(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tables := h.getTableCounts()

	h.render(w, "admin-database.html", map[string]any{
		"Title":         "Database Overview",
		"User":          user,
		"ActiveNav":     "admin",
		"Tables":        tables,
		"Impersonating": h.isImpersonating(r),
	})
}

// TableView renders a table's contents.
func (h *AdminHandler) TableView(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tableName := chi.URLParam(r, "table")
	if !h.isValidTable(tableName) {
		http.Error(w, "Invalid table name", http.StatusBadRequest)
		return
	}

	// Get pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	// Get total count
	var totalCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	h.db.QueryRow(countQuery).Scan(&totalCount)

	// Get columns
	columns, err := h.getTableColumns(tableName)
	if err != nil {
		log.Printf("AdminHandler.TableView error getting columns: %v", err)
		http.Error(w, "Error loading table", http.StatusInternalServerError)
		return
	}

	// Get rows
	rows, err := h.getTableRows(tableName, columns, limit, offset)
	if err != nil {
		log.Printf("AdminHandler.TableView error getting rows: %v", err)
		http.Error(w, "Error loading table", http.StatusInternalServerError)
		return
	}

	totalPages := (totalCount + limit - 1) / limit

	h.render(w, "admin-table.html", map[string]any{
		"Title":         fmt.Sprintf("Table: %s", tableName),
		"User":          user,
		"ActiveNav":     "admin",
		"TableName":     tableName,
		"Columns":       columns,
		"Rows":          rows,
		"CurrentPage":   page,
		"TotalPages":    totalPages,
		"TotalCount":    totalCount,
		"Impersonating": h.isImpersonating(r),
	})
}

// TableRowView renders a single row detail.
func (h *AdminHandler) TableRowView(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tableName := chi.URLParam(r, "table")
	idStr := chi.URLParam(r, "id")

	if !h.isValidTable(tableName) {
		http.Error(w, "Invalid table name", http.StatusBadRequest)
		return
	}

	columns, err := h.getTableColumns(tableName)
	if err != nil {
		http.Error(w, "Error loading table", http.StatusInternalServerError)
		return
	}

	// Get single row
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	row := h.db.QueryRow(query, idStr)

	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := row.Scan(valuePtrs...); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Row not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error loading row", http.StatusInternalServerError)
		return
	}

	// Convert to map
	rowData := make(map[string]interface{})
	for i, col := range columns {
		rowData[col] = values[i]
	}

	h.render(w, "admin-row-detail.html", map[string]any{
		"Title":         fmt.Sprintf("Row %s in %s", idStr, tableName),
		"User":          user,
		"ActiveNav":     "admin",
		"TableName":     tableName,
		"RowID":         idStr,
		"Columns":       columns,
		"RowData":       rowData,
		"Impersonating": h.isImpersonating(r),
	})
}

// SQLQueryPage renders the SQL query interface.
func (h *AdminHandler) SQLQueryPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "admin-sql.html", map[string]any{
		"Title":         "SQL Query",
		"User":          user,
		"ActiveNav":     "admin",
		"Impersonating": h.isImpersonating(r),
	})
}

// SQLQueryExecute executes a SQL query (SELECT only).
func (h *AdminHandler) SQLQueryExecute(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderSQLError(w, user, "", "Invalid form data", h.isImpersonating(r))
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		h.renderSQLError(w, user, "", "Query is required", h.isImpersonating(r))
		return
	}

	// Validate query is SELECT only
	queryUpper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(queryUpper, "SELECT") {
		h.renderSQLError(w, user, query, "Only SELECT queries are allowed", h.isImpersonating(r))
		return
	}

	// Check for dangerous keywords
	dangerous := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE", "GRANT", "REVOKE"}
	for _, keyword := range dangerous {
		if strings.Contains(queryUpper, keyword) {
			h.renderSQLError(w, user, query, "Query contains forbidden keyword: "+keyword, h.isImpersonating(r))
			return
		}
	}

	// Execute query with limit
	if !strings.Contains(queryUpper, "LIMIT") {
		query = query + " LIMIT 1000"
	}

	start := time.Now()
	rows, err := h.db.Query(query)
	duration := time.Since(start)

	if err != nil {
		h.renderSQLError(w, user, query, "Query error: "+err.Error(), h.isImpersonating(r))
		return
	}
	defer rows.Close()

	// Get columns
	columns, err := rows.Columns()
	if err != nil {
		h.renderSQLError(w, user, query, "Error getting columns: "+err.Error(), h.isImpersonating(r))
		return
	}

	// Get rows
	var results [][]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Convert values to strings for display
		rowValues := make([]interface{}, len(columns))
		for i, v := range values {
			if v == nil {
				rowValues[i] = "NULL"
			} else {
				rowValues[i] = fmt.Sprintf("%v", v)
			}
		}
		results = append(results, rowValues)
	}

	h.render(w, "admin-sql.html", map[string]any{
		"Title":         "SQL Query",
		"User":          user,
		"ActiveNav":     "admin",
		"Query":         r.FormValue("query"), // Original query without LIMIT
		"Columns":       columns,
		"Results":       results,
		"RowCount":      len(results),
		"Duration":      duration.String(),
		"Impersonating": h.isImpersonating(r),
	})
}

// Helper methods

func (h *AdminHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

func (h *AdminHandler) renderSQLError(w http.ResponseWriter, user *models.User, query, errMsg string, impersonating bool) {
	h.render(w, "admin-sql.html", map[string]any{
		"Title":         "SQL Query",
		"User":          user,
		"ActiveNav":     "admin",
		"Query":         query,
		"Error":         errMsg,
		"Impersonating": impersonating,
	})
}

func (h *AdminHandler) isImpersonating(r *http.Request) bool {
	_, err := r.Cookie(ImpersonationCookieName)
	return err == nil
}

func (h *AdminHandler) getTableCounts() []map[string]interface{} {
	tables := []string{"users", "sessions", "categories", "accounts", "transactions", "goals", "currency_rates"}
	var result []map[string]interface{}

	for _, table := range tables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		h.db.QueryRow(query).Scan(&count)
		result = append(result, map[string]interface{}{
			"Name":  table,
			"Count": count,
		})
	}

	return result
}

func (h *AdminHandler) isValidTable(name string) bool {
	validTables := map[string]bool{
		"users":          true,
		"sessions":       true,
		"categories":     true,
		"accounts":       true,
		"transactions":   true,
		"goals":          true,
		"currency_rates": true,
	}
	return validTables[name]
}

func (h *AdminHandler) getTableColumns(tableName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := h.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		columns = append(columns, name)
	}

	return columns, nil
}

func (h *AdminHandler) getTableRows(tableName string, columns []string, limit, offset int) ([][]interface{}, error) {
	query := fmt.Sprintf("SELECT * FROM %s ORDER BY id DESC LIMIT %d OFFSET %d", tableName, limit, offset)
	rows, err := h.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results [][]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Convert values for display
		rowValues := make([]interface{}, len(columns))
		for i, v := range values {
			if v == nil {
				rowValues[i] = "NULL"
			} else {
				switch val := v.(type) {
				case []byte:
					rowValues[i] = string(val)
				case time.Time:
					rowValues[i] = val.Format("2006-01-02 15:04:05")
				default:
					rowValues[i] = fmt.Sprintf("%v", v)
				}
			}
		}
		results = append(results, rowValues)
	}

	return results, nil
}
