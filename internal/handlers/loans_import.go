package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/services"
)

// maxImportSize bounds the uploaded CSV to a sane size (4 MB).
const maxImportSize = 4 << 20

// ImportCSV handles a bank-statement upload, parsing it into
// payments/withdrawals on the loan, attributing each via the loan's
// payer rules, and skipping rows already imported.
func (h *LoanHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(maxImportSize); err != nil {
		http.Error(w, "Could not read upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, loanURL(loan.ID, "import=nofile"), http.StatusSeeOther)
		return
	}
	defer file.Close()
	if header.Size > maxImportSize {
		http.Redirect(w, r, loanURL(loan.ID, "import=toobig"), http.StatusSeeOther)
		return
	}

	rows, skipped, err := services.ParseBankCSV(file)
	if err != nil {
		log.Printf("Error parsing import for loan %d: %v", loan.ID, err)
		http.Redirect(w, r, loanURL(loan.ID, "import=parsefail"), http.StatusSeeOther)
		return
	}

	// Default attribution target: the self participant.
	fallbackID := h.selfParticipantID(loan.ID)
	if fallbackID == 0 {
		http.Redirect(w, r, loanURL(loan.ID, "import=noself"), http.StatusSeeOther)
		return
	}

	// Optional: attribute every row to one chosen participant.
	var forceID int64
	if v := strings.TrimSpace(r.FormValue("default_participant_id")); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			forceID = id
		}
	}

	result, err := h.loanService.ImportPayments(loan.ID, rows, skipped, fallbackID, forceID)
	if err != nil {
		log.Printf("Error importing payments for loan %d: %v", loan.ID, err)
		http.Redirect(w, r, loanURL(loan.ID, "import=fail"), http.StatusSeeOther)
		return
	}

	summary := fmt.Sprintf("imported=%d&dup=%d&skipped=%d&unmatched=%d",
		result.Imported, result.Duplicates, result.Skipped, result.Unmatched)
	http.Redirect(w, r, loanURL(loan.ID, summary), http.StatusSeeOther)
}

// AddRule creates a payer rule (description contains X -> participant).
func (h *LoanHandler) AddRule(w http.ResponseWriter, r *http.Request) {
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

	matchText := strings.TrimSpace(r.FormValue("match_text"))
	if matchText == "" {
		http.Error(w, "Match text is required", http.StatusBadRequest)
		return
	}
	participantID, err := strconv.ParseInt(r.FormValue("participant_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid participant", http.StatusBadRequest)
		return
	}
	// Ensure the participant belongs to this loan.
	p, err := h.loanRepo.GetParticipantByID(participantID)
	if err != nil || p == nil || p.LoanID != loan.ID {
		http.Error(w, "Participant not found", http.StatusNotFound)
		return
	}

	if _, err := h.loanRepo.AddImportRule(&models.LoanImportRule{
		LoanID:        loan.ID,
		MatchText:     matchText,
		ParticipantID: participantID,
	}); err != nil {
		log.Printf("Error adding import rule: %v", err)
		http.Error(w, "Failed to add rule", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// DeleteRule removes a payer rule.
func (h *LoanHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	loan, ok := h.ownedLoan(w, r, user)
	if !ok {
		return
	}

	ruleID, err := strconv.ParseInt(chi.URLParam(r, "ruleID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}
	rule, err := h.loanRepo.GetImportRuleByID(ruleID)
	if err != nil || rule == nil || rule.LoanID != loan.ID {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}

	if err := h.loanRepo.DeleteImportRule(ruleID); err != nil {
		log.Printf("Error deleting import rule: %v", err)
		http.Error(w, "Failed to delete rule", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// selfParticipantID returns the loan's self participant id, or 0.
func (h *LoanHandler) selfParticipantID(loanID int64) int64 {
	participants, err := h.loanRepo.GetParticipants(loanID)
	if err != nil {
		return 0
	}
	for _, p := range participants {
		if p.IsSelf {
			return p.ID
		}
	}
	return 0
}

// loanURL builds a loan detail URL with a query string (for the import
// summary banner).
func loanURL(loanID int64, query string) string {
	u := "/loans/" + strconv.FormatInt(loanID, 10)
	if query != "" {
		u += "?" + query
	}
	return u
}
