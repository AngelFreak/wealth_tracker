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
	"wealth_tracker/internal/services"
)

// LoanHandler handles loan routes: loans the user owes, has lent out, or
// co-owns (split). The settlement and net-worth math lives in the
// LoanService; this handler is the HTTP/form layer around it.
type LoanHandler struct {
	templates    map[string]*template.Template
	loanRepo     *repository.LoanRepository
	loanService  *services.LoanService
	categoryRepo *repository.CategoryRepository
}

// NewLoanHandler creates a new LoanHandler.
func NewLoanHandler(
	templates map[string]*template.Template,
	loanRepo *repository.LoanRepository,
	loanService *services.LoanService,
	categoryRepo *repository.CategoryRepository,
) *LoanHandler {
	return &LoanHandler{
		templates:    templates,
		loanRepo:     loanRepo,
		loanService:  loanService,
		categoryRepo: categoryRepo,
	}
}

// parseCategoryID reads a category_id form value and returns it only if
// the category exists and belongs to the user; otherwise nil.
func (h *LoanHandler) parseCategoryID(value string, userID int64) *int64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	if h.categoryRepo == nil {
		return &id
	}
	cat, _ := h.categoryRepo.GetByID(id)
	if cat == nil || cat.UserID != userID {
		return nil
	}
	return &id
}

// sync keeps a loan's managed accounts in step after a change; errors are
// logged but not fatal to the user's action.
func (h *LoanHandler) sync(loanID int64) {
	if err := h.loanService.SyncManagedAccounts(loanID); err != nil {
		log.Printf("Error syncing managed accounts for loan %d: %v", loanID, err)
	}
}

// List renders the loans overview page.
func (h *LoanHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	loans, err := h.loanRepo.GetByUserID(user.ID)
	if err != nil {
		log.Printf("Error fetching loans: %v", err)
		http.Error(w, "Error loading loans", http.StatusInternalServerError)
		return
	}

	// Build a summary for each loan so the cards can show remaining,
	// equity, and the self balance.
	summaries := make([]*models.LoanSummary, 0, len(loans))
	for _, loan := range loans {
		s, err := h.loanService.Summarize(loan.ID)
		if err != nil {
			log.Printf("Error summarizing loan %d: %v", loan.ID, err)
			continue
		}
		summaries = append(summaries, s)
	}

	// Totals for the header cards. These summarise ALL of the user's
	// loans (including categorised ones that surface as accounts), since
	// this is a loans overview — not the dashboard net-worth fold, which
	// deliberately excludes categorised loans to avoid double-counting.
	var assets, liabilities, receivable float64
	for _, s := range summaries {
		if s.Loan != nil && !s.Loan.IsActive {
			continue
		}
		assets += s.SelfAsset
		liabilities += s.SelfLoanShare
		receivable += s.SelfReceivable
	}

	categories, _ := h.categoryRepo.GetByUserID(user.ID)

	h.render(w, "loans.html", map[string]any{
		"Title":           "Loans",
		"User":            user,
		"ActiveNav":       "loans",
		"Summaries":       summaries,
		"Categories":      categories,
		"TotalAssets":     assets,
		"TotalLiability":  liabilities,
		"NetReceivable":   receivable,
		"NetContribution": assets - liabilities + receivable,
		"DemoMode":        IsDemoMode(),
	})
}

// Detail renders a single loan with its participants, settlement
// balances, and payments.
func (h *LoanHandler) Detail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	// Keep the managed accounts in step with the current math on view, so
	// they self-heal after a code change to the calculation (sync
	// otherwise only runs on a mutation).
	h.sync(loan.ID)

	summary, err := h.loanService.Summarize(loan.ID)
	if err != nil {
		log.Printf("Error summarizing loan %d: %v", loan.ID, err)
		http.Error(w, "Error loading loan", http.StatusInternalServerError)
		return
	}

	payments, err := h.loanRepo.GetPayments(loan.ID)
	if err != nil {
		log.Printf("Error fetching payments: %v", err)
		http.Error(w, "Error loading payments", http.StatusInternalServerError)
		return
	}

	participants, err := h.loanRepo.GetParticipants(loan.ID)
	if err != nil {
		log.Printf("Error fetching participants: %v", err)
		http.Error(w, "Error loading participants", http.StatusInternalServerError)
		return
	}

	// Map participant IDs to names so the payments table can show who paid.
	participantNames := make(map[int64]string, len(participants))
	for _, p := range participants {
		participantNames[p.ID] = p.Name
	}

	categories, _ := h.categoryRepo.GetByUserID(user.ID)
	rules, _ := h.loanRepo.GetImportRules(loan.ID)

	h.render(w, "loan-detail.html", map[string]any{
		"Title":            loan.Name,
		"User":             user,
		"ActiveNav":        "loans",
		"Loan":             loan,
		"Summary":          summary,
		"Participants":     participants,
		"Payments":         payments,
		"ParticipantNames": participantNames,
		"Categories":       categories,
		"Rules":            rules,
		"ImportSummary":    importSummaryFromQuery(r),
		"DemoMode":         IsDemoMode(),
	})
}

// importSummaryFromQuery turns the ?imported=&dup=&… params left by an
// import redirect into a short human message, or "" if none.
func importSummaryFromQuery(r *http.Request) string {
	q := r.URL.Query()
	switch q.Get("import") {
	case "nofile":
		return "No file was selected."
	case "toobig":
		return "That file is too large to import."
	case "parsefail":
		return "The file could not be read as a bank CSV."
	case "noself":
		return "Add yourself as a participant before importing."
	case "fail":
		return "The import failed. Please try again."
	}
	if q.Get("imported") == "" {
		return ""
	}
	imported := q.Get("imported")
	dup := q.Get("dup")
	skipped := q.Get("skipped")
	unmatched := q.Get("unmatched")
	msg := "Imported " + imported + " postings"
	if dup != "" && dup != "0" {
		msg += ", skipped " + dup + " duplicates"
	}
	if skipped != "" && skipped != "0" {
		msg += ", ignored " + skipped + " unparseable rows"
	}
	if unmatched != "" && unmatched != "0" {
		msg += ". " + unmatched + " assigned to you (no rule matched)"
	}
	return msg + "."
}

// Create handles creating a new loan. A self participant is created
// automatically so the loan immediately counts toward net worth; for a
// split loan the user adds co-owners afterwards on the detail page.
func (h *LoanHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	loanType := strings.TrimSpace(r.FormValue("loan_type"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	principal := parseFloat(r.FormValue("principal"))
	propertyValue := parseFloat(r.FormValue("property_value"))
	interestRate := parseFloat(r.FormValue("interest_rate"))

	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if !validLoanType(loanType) {
		loanType = models.LoanTypeOwed
	}
	if currency == "" {
		currency = "DKK"
	}

	var startDate *time.Time
	if sd := strings.TrimSpace(r.FormValue("start_date")); sd != "" {
		if t, err := time.Parse("2006-01-02", sd); err == nil {
			startDate = &t
		}
	}

	loan := &models.Loan{
		UserID:        user.ID,
		Name:          name,
		LoanType:      loanType,
		Principal:     principal,
		PropertyValue: propertyValue,
		Currency:      currency,
		InterestRate:  interestRate,
		StartDate:     startDate,
		IsActive:      true,
		Notes:         notes,
		CategoryID:    h.parseCategoryID(r.FormValue("category_id"), user.ID),
	}

	loanID, err := h.loanRepo.Create(loan)
	if err != nil {
		log.Printf("Error creating loan: %v", err)
		http.Error(w, "Failed to create loan", http.StatusInternalServerError)
		return
	}

	// Create the self participant. For a simple loan they own 100%; for a
	// split loan the user adjusts this and adds co-owners on the detail
	// page (ownership is validated to sum to 100 there).
	selfName := strings.TrimSpace(r.FormValue("self_name"))
	if selfName == "" {
		selfName = "Me"
	}
	selfPct := parseFloat(r.FormValue("self_ownership_pct"))
	if selfPct <= 0 {
		selfPct = 100
	}
	if _, err := h.loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID:       loanID,
		Name:         selfName,
		OwnershipPct: selfPct,
		IsSelf:       true,
	}); err != nil {
		log.Printf("Error creating self participant for loan %d: %v", loanID, err)
	}

	h.sync(loanID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loanID, 10), http.StatusSeeOther)
}

// Update handles editing a loan's core fields.
func (h *LoanHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	if r.FormValue("_method") == "DELETE" {
		h.Delete(w, r)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	loanType := strings.TrimSpace(r.FormValue("loan_type"))
	if validLoanType(loanType) {
		loan.LoanType = loanType
	}
	loan.Name = name
	loan.Principal = parseFloat(r.FormValue("principal"))
	loan.PropertyValue = parseFloat(r.FormValue("property_value"))
	loan.InterestRate = parseFloat(r.FormValue("interest_rate"))
	loan.Notes = strings.TrimSpace(r.FormValue("notes"))
	loan.IsActive = r.FormValue("is_active") != "0"
	if currency := strings.TrimSpace(r.FormValue("currency")); currency != "" {
		loan.Currency = currency
	}
	if sd := strings.TrimSpace(r.FormValue("start_date")); sd != "" {
		if t, err := time.Parse("2006-01-02", sd); err == nil {
			loan.StartDate = &t
		}
	}
	loan.CategoryID = h.parseCategoryID(r.FormValue("category_id"), user.ID)

	if err := h.loanRepo.Update(loan); err != nil {
		log.Printf("Error updating loan: %v", err)
		http.Error(w, "Failed to update loan", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// Delete removes a loan (and its participants/payments via cascade).
func (h *LoanHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	if err := h.loanRepo.Delete(loan.ID); err != nil {
		log.Printf("Error deleting loan: %v", err)
		http.Error(w, "Failed to delete loan", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/loans", http.StatusSeeOther)
}

// AddParticipant adds a co-owner to a loan.
func (h *LoanHandler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Participant name is required", http.StatusBadRequest)
		return
	}
	pct := parseFloat(r.FormValue("ownership_pct"))

	if _, err := h.loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID:       loan.ID,
		Name:         name,
		OwnershipPct: pct,
		IsSelf:       false,
	}); err != nil {
		log.Printf("Error adding participant: %v", err)
		http.Error(w, "Failed to add participant", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// DeleteParticipant removes a co-owner from a loan. The self participant
// cannot be deleted (it anchors the net-worth contribution).
func (h *LoanHandler) DeleteParticipant(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	participantID, err := strconv.ParseInt(chi.URLParam(r, "participantID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid participant ID", http.StatusBadRequest)
		return
	}

	participant, err := h.loanRepo.GetParticipantByID(participantID)
	if err != nil || participant == nil || participant.LoanID != loan.ID {
		http.Error(w, "Participant not found", http.StatusNotFound)
		return
	}
	if participant.IsSelf {
		http.Error(w, "Cannot remove yourself from a loan", http.StatusBadRequest)
		return
	}

	if err := h.loanRepo.DeleteParticipant(participantID); err != nil {
		log.Printf("Error deleting participant: %v", err)
		http.Error(w, "Failed to delete participant", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// RecordPayment records a payment made by a participant toward a loan.
func (h *LoanHandler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// "shared" attributes the payment to everyone by ownership %. The
	// participant_id column is NOT NULL, so we store the self participant
	// as a placeholder; the settlement math ignores it while shared.
	isShared := r.FormValue("participant_id") == "shared"
	var participantID int64
	if isShared {
		participantID = h.selfParticipantID(loan.ID)
		if participantID == 0 {
			http.Error(w, "Add yourself as a participant first", http.StatusBadRequest)
			return
		}
	} else {
		var err error
		participantID, err = strconv.ParseInt(r.FormValue("participant_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid participant", http.StatusBadRequest)
			return
		}
		// Ensure the participant belongs to this loan.
		participant, err := h.loanRepo.GetParticipantByID(participantID)
		if err != nil || participant == nil || participant.LoanID != loan.ID {
			http.Error(w, "Participant not found", http.StatusNotFound)
			return
		}
	}

	amount := parseFloat(r.FormValue("amount"))
	if amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	paymentType := strings.TrimSpace(r.FormValue("payment_type"))
	if !validPaymentType(paymentType) {
		paymentType = models.PaymentTypeRegular
	}

	paymentDate := time.Now()
	if pd := strings.TrimSpace(r.FormValue("payment_date")); pd != "" {
		if t, err := time.Parse("2006-01-02", pd); err == nil {
			paymentDate = t
		}
	}

	if _, err := h.loanRepo.AddPayment(&models.LoanPayment{
		LoanID:        loan.ID,
		ParticipantID: participantID,
		Amount:        amount,
		PaymentType:   paymentType,
		PaymentDate:   paymentDate,
		Description:   strings.TrimSpace(r.FormValue("description")),
		IsShared:      isShared,
	}); err != nil {
		log.Printf("Error recording payment: %v", err)
		http.Error(w, "Failed to record payment", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// DeletePayment removes a payment from a loan.
func (h *LoanHandler) DeletePayment(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	paymentID, err := strconv.ParseInt(chi.URLParam(r, "paymentID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid payment ID", http.StatusBadRequest)
		return
	}

	payment, err := h.loanRepo.GetPaymentByID(paymentID)
	if err != nil || payment == nil || payment.LoanID != loan.ID {
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	if err := h.loanRepo.DeletePayment(paymentID); err != nil {
		log.Printf("Error deleting payment: %v", err)
		http.Error(w, "Failed to delete payment", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// UpdatePaymentPayer reassigns a payment to a different participant,
// correcting who-paid (e.g. after an import attributed everything to the
// self participant).
func (h *LoanHandler) UpdatePaymentPayer(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	paymentID, err := strconv.ParseInt(chi.URLParam(r, "paymentID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid payment ID", http.StatusBadRequest)
		return
	}
	payment, err := h.loanRepo.GetPaymentByID(paymentID)
	if err != nil || payment == nil || payment.LoanID != loan.ID {
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	// "shared" attributes the payment to everyone by ownership %.
	if r.FormValue("participant_id") == "shared" {
		if err := h.loanRepo.SetPaymentShared(paymentID); err != nil {
			log.Printf("Error sharing payment: %v", err)
			http.Error(w, "Failed to update payer", http.StatusInternalServerError)
			return
		}
		h.sync(loan.ID)
		http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
		return
	}

	participantID, err := strconv.ParseInt(r.FormValue("participant_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid participant", http.StatusBadRequest)
		return
	}
	participant, err := h.loanRepo.GetParticipantByID(participantID)
	if err != nil || participant == nil || participant.LoanID != loan.ID {
		http.Error(w, "Participant not found", http.StatusNotFound)
		return
	}

	if err := h.loanRepo.UpdatePaymentParticipant(paymentID, participantID); err != nil {
		log.Printf("Error updating payment payer: %v", err)
		http.Error(w, "Failed to update payer", http.StatusInternalServerError)
		return
	}

	h.sync(loan.ID)
	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// ownedLoan loads the loan named by the {id} URL param and verifies it
// belongs to the user. It writes the appropriate HTTP error and returns
// ok=false on any failure.
func (h *LoanHandler) ownedLoan(w http.ResponseWriter, r *http.Request, user *models.User) (*models.Loan, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid loan ID", http.StatusBadRequest)
		return nil, false
	}
	loan, err := h.loanRepo.GetByID(id)
	if err != nil || loan == nil {
		http.Error(w, "Loan not found", http.StatusNotFound)
		return nil, false
	}
	if loan.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return nil, false
	}
	return loan, true
}

// render renders a template with the given data.
func (h *LoanHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// parseFloat parses a form value into a float64, returning 0 on error.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func validLoanType(t string) bool {
	switch t {
	case models.LoanTypeOwed, models.LoanTypeLent, models.LoanTypeSplit:
		return true
	}
	return false
}

func validPaymentType(t string) bool {
	switch t {
	case models.PaymentTypeDownPayment, models.PaymentTypeRegular, models.PaymentTypeExtra:
		return true
	}
	return false
}
