package handlers

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"wealth_tracker/internal/broker/nordnet"
	"wealth_tracker/internal/broker/saxo"
	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
	"wealth_tracker/internal/sync"
)

// BrokerHandler handles broker connection routes.
type BrokerHandler struct {
	templates   map[string]*template.Template
	connRepo    *repository.BrokerConnectionRepository
	mappingRepo *repository.AccountMappingRepository
	holdingRepo *repository.HoldingRepository
	historyRepo *repository.SyncHistoryRepository
	accountRepo *repository.AccountRepository
	syncService *sync.Service
}

// NewBrokerHandler creates a new BrokerHandler.
func NewBrokerHandler(
	templates map[string]*template.Template,
	connRepo *repository.BrokerConnectionRepository,
	mappingRepo *repository.AccountMappingRepository,
	holdingRepo *repository.HoldingRepository,
	historyRepo *repository.SyncHistoryRepository,
	accountRepo *repository.AccountRepository,
	syncService *sync.Service,
) *BrokerHandler {
	return &BrokerHandler{
		templates:   templates,
		connRepo:    connRepo,
		mappingRepo: mappingRepo,
		holdingRepo: holdingRepo,
		historyRepo: historyRepo,
		accountRepo: accountRepo,
		syncService: syncService,
	}
}

// Connections lists all broker connections for the user.
func (h *BrokerHandler) Connections(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	connections, err := h.connRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching broker connections: %v", err)
		h.render(w, "connections.html", map[string]any{
			"Title":     "Connections",
			"User":      user,
			"ActiveNav": "settings",
			"Error":     "Failed to load connections",
		})
		return
	}

	h.render(w, "connections.html", map[string]any{
		"Title":       "Connections",
		"User":        user,
		"ActiveNav":   "settings",
		"Connections": connections,
	})
}

// NewConnectionForm shows the form to add a new broker connection.
func (h *BrokerHandler) NewConnectionForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "connection-form.html", map[string]any{
		"Title":     "Add Connection",
		"User":      user,
		"ActiveNav": "settings",
		"IsNew":     true,
	})
}

// CreateConnection creates a new broker connection.
// For Nordnet (MitID), no password is stored - user authenticates interactively each sync.
// For Saxo (OAuth), no credentials are stored - user authenticates via browser.
func (h *BrokerHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderConnectionForm(w, user, true, nil, "Invalid form data")
		return
	}

	brokerType := strings.TrimSpace(r.FormValue("broker_type"))
	country := strings.TrimSpace(r.FormValue("country"))
	username := strings.TrimSpace(r.FormValue("username"))       // For Nordnet: MitID user identifier
	cpr := strings.TrimSpace(r.FormValue("cpr"))                 // For Nordnet: CPR number for Signicat
	appKey := strings.TrimSpace(r.FormValue("app_key"))           // For Saxo: App Key (client_id)
	appSecret := strings.TrimSpace(r.FormValue("app_secret"))     // For Saxo: App Secret (client_secret)
	redirectURI := strings.TrimSpace(r.FormValue("redirect_uri")) // For Saxo: OAuth redirect URI

	// Validate broker type and country
	if brokerType == "" || country == "" {
		h.renderConnectionForm(w, user, true, nil, "Broker type and country are required")
		return
	}

	// Broker-specific validation
	switch brokerType {
	case "nordnet":
		// Nordnet requires MitID credentials
		if username == "" {
			h.renderConnectionForm(w, user, true, nil, "MitID user ID is required for Nordnet")
			return
		}
		// Validate CPR format (10 digits) for Danish Nordnet
		if country == "dk" {
			if len(cpr) != 10 {
				h.renderConnectionForm(w, user, true, nil, "CPR number must be 10 digits")
				return
			}
			for _, c := range cpr {
				if c < '0' || c > '9' {
					h.renderConnectionForm(w, user, true, nil, "CPR number must contain only digits")
					return
				}
			}
		}
	case "saxo":
		// Saxo requires App Key and Redirect URI for OAuth
		if appKey == "" {
			h.renderConnectionForm(w, user, true, nil, "Saxo App Key is required")
			return
		}
		if redirectURI == "" {
			h.renderConnectionForm(w, user, true, nil, "Saxo Redirect URI is required")
			return
		}
	default:
		h.renderConnectionForm(w, user, true, nil, "Unsupported broker type")
		return
	}

	// Check if connection already exists
	existing, _ := h.connRepo.GetByUserAndBroker(user.ID, brokerType)
	if existing != nil {
		h.renderConnectionForm(w, user, true, nil, "You already have a connection for this broker")
		return
	}

	// Test connection (no-op for interactive auth brokers, validates broker type)
	if err := h.syncService.TestConnection(brokerType, country, username, ""); err != nil {
		h.renderConnectionForm(w, user, true, nil, "Invalid broker configuration: "+err.Error())
		return
	}

	// Create connection
	conn := &models.BrokerConnection{
		UserID:      user.ID,
		BrokerType:  brokerType,
		Username:    username,    // Stores MitID user identifier (empty for Saxo)
		CPR:         cpr,         // Stores CPR for Signicat verification (empty for Saxo)
		AppKey:      appKey,      // Stores Saxo App Key (empty for Nordnet)
		AppSecret:   appSecret,   // Stores Saxo App Secret (empty for Nordnet and PKCE apps)
		RedirectURI: redirectURI, // Stores Saxo OAuth redirect URI (empty for Nordnet)
		Country:     country,
		IsActive:    true,
	}

	id, err := h.connRepo.Create(conn)
	if err != nil {
		log.Printf("Error creating broker connection: %v", err)
		h.renderConnectionForm(w, user, true, nil, "Failed to save connection")
		return
	}

	// Redirect to connection detail page
	http.Redirect(w, r, "/settings/connections/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// EditConnectionForm shows the form to edit a broker connection.
func (h *BrokerHandler) EditConnectionForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Extract ID from URL path: /settings/connections/{id}/edit
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/edit")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(id)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	h.render(w, "connection-form.html", map[string]any{
		"Title":      "Edit Connection",
		"User":       user,
		"ActiveNav":  "settings",
		"IsNew":      false,
		"Connection": conn,
	})
}

// UpdateConnection updates an existing broker connection.
func (h *BrokerHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract ID from URL path: /settings/connections/{id}/edit
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/edit")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(id)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderConnectionForm(w, user, false, conn, "Invalid form data")
		return
	}

	// Update fields based on broker type
	if conn.BrokerType == "nordnet" {
		conn.Username = strings.TrimSpace(r.FormValue("username"))
		conn.CPR = strings.TrimSpace(r.FormValue("cpr"))

		// Validate
		if conn.Username == "" {
			h.renderConnectionForm(w, user, false, conn, "MitID user ID is required for Nordnet")
			return
		}
		if conn.Country == "dk" && len(conn.CPR) != 10 {
			h.renderConnectionForm(w, user, false, conn, "CPR number must be 10 digits")
			return
		}
	} else if conn.BrokerType == "saxo" {
		conn.AppKey = strings.TrimSpace(r.FormValue("app_key"))
		conn.AppSecret = strings.TrimSpace(r.FormValue("app_secret"))
		conn.RedirectURI = strings.TrimSpace(r.FormValue("redirect_uri"))

		// Validate
		if conn.AppKey == "" {
			h.renderConnectionForm(w, user, false, conn, "Saxo App Key is required")
			return
		}
		if conn.RedirectURI == "" {
			h.renderConnectionForm(w, user, false, conn, "Saxo Redirect URI is required")
			return
		}
	}

	// Update in database
	if err := h.connRepo.Update(conn); err != nil {
		log.Printf("Error updating broker connection: %v", err)
		h.renderConnectionForm(w, user, false, conn, "Failed to update connection")
		return
	}

	// Clear any cached sessions since credentials may have changed
	if conn.BrokerType == "saxo" {
		saxo.ClearCachedSession(conn.ID)
		saxo.ClearActiveOAuthSession(conn.ID)
	}

	// Redirect back to connection detail page
	http.Redirect(w, r, "/settings/connections/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// ViewConnection shows details of a specific connection.
func (h *BrokerHandler) ViewConnection(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Extract ID from URL path
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.Split(idStr, "/")[0]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(id)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	mappings, _ := h.mappingRepo.GetByConnectionID(id)
	history, _ := h.historyRepo.GetByConnectionID(id, 10)

	h.render(w, "connection-detail.html", map[string]any{
		"Title":      "Connection Details",
		"User":       user,
		"ActiveNav":  "settings",
		"Connection": conn,
		"Mappings":   mappings,
		"History":    history,
	})
}

// AccountMappingForm shows the form to map broker accounts to local accounts.
// Initially shows a loading state, accounts are fetched via AJAX to allow QR code display.
func (h *BrokerHandler) AccountMappingForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Extract connection ID from URL
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/accounts")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(id)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	// Get existing mappings
	existingMappings, _ := h.mappingRepo.GetByConnectionID(id)
	mappingsByExternal := make(map[string]*models.AccountMapping)
	for _, m := range existingMappings {
		mappingsByExternal[m.ExternalAccountID] = m
	}

	// Convert mappings to JSON for JavaScript
	mappingsJSON, _ := json.Marshal(mappingsByExternal)

	// Get local accounts for dropdown
	localAccounts, _ := h.accountRepo.GetByUserIDActiveOnly(user.ID)

	// Render page immediately - accounts will be fetched via AJAX
	h.render(w, "account-mapping.html", map[string]any{
		"Title":               "Map Accounts",
		"User":                user,
		"ActiveNav":           "settings",
		"Connection":          conn,
		"ExternalAccounts":    nil, // Will be fetched via AJAX
		"ExistingMappings":    mappingsByExternal,
		"ExistingMappingsJSON": template.JS(mappingsJSON), // Safe for embedding in JavaScript
		"LocalAccounts":       localAccounts,
		"NeedsFetch":          true, // Flag to trigger AJAX fetch
	})
}

// FetchExternalAccounts fetches accounts from broker via AJAX (triggers MitID auth).
func (h *BrokerHandler) FetchExternalAccounts(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID from URL
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/fetch-accounts")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid connection ID", http.StatusBadRequest)
		return
	}

	conn, err := h.connRepo.GetByID(id)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	// Fetch external accounts from broker (this triggers MitID auth)
	externalAccounts, err := h.syncService.GetExternalAccounts(id)
	if err != nil {
		log.Printf("Error fetching external accounts: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return accounts as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(externalAccounts)
}

// SaveAccountMappings saves account mappings.
func (h *BrokerHandler) SaveAccountMappings(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Extract connection ID
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/accounts")
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	// Process mappings from form
	// Form fields: mapping_<external_account_id> = <local_account_id>
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "mapping_") || len(values) == 0 {
			continue
		}

		externalAccountID := strings.TrimPrefix(key, "mapping_")
		localAccountIDStr := values[0]

		if localAccountIDStr == "" || localAccountIDStr == "0" {
			// Remove mapping if exists
			existing, _ := h.mappingRepo.GetByExternalAccountID(connectionID, externalAccountID)
			if existing != nil {
				h.mappingRepo.Delete(existing.ID)
			}
			continue
		}

		localAccountID, err := strconv.ParseInt(localAccountIDStr, 10, 64)
		if err != nil {
			continue
		}

		// Verify local account belongs to user
		localAccount, _ := h.accountRepo.GetByID(localAccountID)
		if localAccount == nil || localAccount.UserID != user.ID {
			continue
		}

		// Check if mapping exists
		existing, _ := h.mappingRepo.GetByExternalAccountID(connectionID, externalAccountID)
		if existing != nil {
			// Update existing
			existing.LocalAccountID = localAccountID
			h.mappingRepo.Update(existing)
		} else {
			// Create new mapping
			externalName := r.FormValue("name_" + externalAccountID)
			mapping := &models.AccountMapping{
				ConnectionID:        connectionID,
				LocalAccountID:      localAccountID,
				ExternalAccountID:   externalAccountID,
				ExternalAccountName: externalName,
				AutoSync:            true,
			}
			h.mappingRepo.Create(mapping)
		}
	}

	// Redirect to connection details
	http.Redirect(w, r, "/settings/connections/"+strconv.FormatInt(connectionID, 10), http.StatusSeeOther)
}

// SyncConnection triggers a manual sync for a connection.
func (h *BrokerHandler) SyncConnection(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/sync")
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	// Run sync
	result, err := h.syncService.SyncConnection(connectionID)
	if err != nil {
		log.Printf("Error syncing connection %d: %v", connectionID, err)
		// Return error via HTMX
		w.Header().Set("HX-Trigger", `{"showToast": "Sync failed: `+err.Error()+`"}`)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success via HTMX
	w.Header().Set("HX-Trigger", `{"showToast": "Synced `+strconv.Itoa(result.PositionsSynced)+` positions from `+strconv.Itoa(result.AccountsSynced)+` accounts"}`)
	w.WriteHeader(http.StatusOK)
}

// DeleteConnection removes a broker connection.
func (h *BrokerHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.TrimSuffix(idStr, "/delete")
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	// Delete connection (cascades to mappings, sessions, and history)
	if err := h.connRepo.Delete(connectionID); err != nil {
		log.Printf("Error deleting connection %d: %v", connectionID, err)
		http.Error(w, "Failed to delete connection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings/connections", http.StatusSeeOther)
}

// render renders a template with the given data.
func (h *BrokerHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// renderConnectionForm re-renders the connection form with an error.
func (h *BrokerHandler) renderConnectionForm(w http.ResponseWriter, user any, isNew bool, conn *models.BrokerConnection, errMsg string) {
	h.render(w, "connection-form.html", map[string]any{
		"Title":      "Add Connection",
		"User":       user,
		"ActiveNav":  "settings",
		"IsNew":      isNew,
		"Connection": conn,
		"Error":      errMsg,
	})
}

// MitIDStatus returns the current status of MitID authentication for a connection.
func (h *BrokerHandler) MitIDStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID from URL: /settings/connections/{id}/mitid/status
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.Split(idStr, "/")[0]
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	status := nordnet.GetMitIDStatusNative(connectionID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

// MitIDQRCode serves the current QR code image for MitID authentication.
func (h *BrokerHandler) MitIDQRCode(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID from URL: /settings/connections/{id}/mitid/qr
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.Split(idStr, "/")[0]
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	qrPath, err := nordnet.GetQRCodePathNative(connectionID)
	if err != nil {
		// Return a placeholder or error status
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Read and serve the QR code image
	data, err := os.ReadFile(qrPath)
	if err != nil {
		log.Printf("Error reading QR code: %v", err)
		http.Error(w, "QR code not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(data)
}

// SaxoOAuthStatus returns the current status of OAuth authentication for a Saxo connection.
func (h *BrokerHandler) SaxoOAuthStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID from URL: /settings/connections/{id}/saxo/status
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.Split(idStr, "/")[0]
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	if conn.BrokerType != "saxo" {
		http.Error(w, "Not a Saxo connection", http.StatusBadRequest)
		return
	}

	status := saxo.GetOAuthStatus(connectionID)
	authURL := saxo.GetOAuthURL(connectionID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   status,
		"auth_url": authURL,
	})
}

// SaxoStartOAuth initiates OAuth authentication for a Saxo connection.
func (h *BrokerHandler) SaxoStartOAuth(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract connection ID from URL: /settings/connections/{id}/saxo/auth
	idStr := strings.TrimPrefix(r.URL.Path, "/settings/connections/")
	idStr = strings.Split(idStr, "/")[0]
	connectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	conn, err := h.connRepo.GetByID(connectionID)
	if err != nil || conn == nil || conn.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	if conn.BrokerType != "saxo" {
		http.Error(w, "Not a Saxo connection", http.StatusBadRequest)
		return
	}

	// Start OAuth flow in background
	if err := h.syncService.StartSaxoOAuth(connectionID); err != nil {
		log.Printf("Error starting Saxo OAuth: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "started",
	})
}
