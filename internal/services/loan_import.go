package services

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"wealth_tracker/internal/models"
)

// ImportedRow is one parsed posting from a bank CSV, before it is
// attributed to a participant and written.
type ImportedRow struct {
	Date        time.Time
	Amount      float64 // signed: positive = payment, negative = withdrawal
	Description string
	Hash        string // dedup key = sha256(date|amount|description)
}

// ImportResult summarises an import run for the user.
type ImportResult struct {
	Imported   int      // rows written
	Duplicates int      // rows skipped because already present
	Skipped    int      // rows that couldn't be parsed
	Unmatched  int      // rows no rule matched, left unassigned for a manual pick
	Errors     []string // non-fatal problems, for display
}

// ParseBankCSV parses a Danish bank statement export (as produced in
// "Posteringsdetaljer.csv"): UTF-8 with an optional BOM, semicolon
// delimited, NO header row. The relevant columns are:
//
//	[1] description/text   (e.g. "Fra Teis", "Betaling andelslån")
//	[4] amount             Danish format, e.g. "4.000,00" or "-800,00"
//	[7] booking date       DD-MM-YYYY
//
// A positive amount is treated as a payment into the loan; a negative
// amount as a withdrawal (borrowing more). Rows whose amount or date
// can't be parsed are skipped and counted by the caller.
func ParseBankCSV(r io.Reader) ([]ImportedRow, int, error) {
	reader := csv.NewReader(stripBOM(r))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1 // rows have a trailing-semicolon variable count
	reader.LazyQuotes = true

	var rows []ImportedRow
	skipped := 0
	seen := make(map[string]int) // base hash -> occurrences so far in this file

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A malformed line shouldn't abort the whole import.
			skipped++
			continue
		}

		description := ""
		if len(record) > 1 {
			description = strings.TrimSpace(record[1])
		}
		amountStr := ""
		if len(record) > 4 {
			amountStr = record[4]
		}
		dateStr := ""
		if len(record) > 7 {
			dateStr = strings.TrimSpace(record[7])
		}

		amount, ok := parseDanishAmount(amountStr)
		if !ok {
			skipped++
			continue
		}
		date, ok := parseDanishDate(dateStr)
		if !ok {
			skipped++
			continue
		}

		// Banks can post two genuinely identical rows (same date, amount
		// and text). Number repeats within the file so each gets its own
		// key; the first keeps the plain hash so earlier imports still match.
		hash := importHash(date, amount, description)
		if n := seen[hash]; n > 0 {
			seen[hash] = n + 1
			hash = occurrenceHash(hash, n)
		} else {
			seen[hash] = 1
		}

		rows = append(rows, ImportedRow{
			Date:        date,
			Amount:      amount,
			Description: description,
			Hash:        hash,
		})
	}

	return rows, skipped, nil
}

// stripBOM returns a reader with a leading UTF-8 BOM removed if present.
func stripBOM(r io.Reader) io.Reader {
	data, err := io.ReadAll(r)
	if err != nil {
		return strings.NewReader("")
	}
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
	return strings.NewReader(string(data))
}

// parseDanishAmount parses "4.000,00", "-800,00", "1.305,50" → float.
// Danish format uses '.' as the thousands separator and ',' as the
// decimal separator. Returns ok=false for blank or unparseable values.
func parseDanishAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Remove thousands separators, then swap decimal comma for a dot.
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")

	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
		return 0, false
	}
	return f, true
}

// parseDanishDate parses "DD-MM-YYYY". Returns ok=false otherwise.
func parseDanishDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("02-01-2006", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// importHash builds the dedup key for a posting. Two postings are "the
// same" when their date, amount, and description match.
func importHash(date time.Time, amount float64, description string) string {
	key := fmt.Sprintf("%s|%.2f|%s", date.Format("2006-01-02"), amount, strings.TrimSpace(description))
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// occurrenceHash derives the dedup key for the nth (n >= 1) repeat of an
// identical posting within one file.
func occurrenceHash(base string, n int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", base, n)))
	return hex.EncodeToString(sum[:])
}

// ImportPayments writes parsed rows to a loan as payments/withdrawals,
// attributing each to a participant and skipping rows already imported.
//
// If forceParticipantID is non-zero, every row is attributed to that
// participant (the user chose a single payer at import time). Otherwise
// each row is matched against the loan's payer rules; rows no rule
// matches are left unassigned (NeedsAssignment) for the user to pick a
// payer, rather than guessed. placeholderParticipantID fills the
// participant_id column of shared and unassigned rows, where it is
// ignored.
//
// rows are typically the output of ParseBankCSV; skippedParse is the
// count it reported so the final summary is complete.
func (s *LoanService) ImportPayments(loanID int64, rows []ImportedRow, skippedParse int, placeholderParticipantID, forceParticipantID int64) (*ImportResult, error) {
	rules, err := s.loanRepo.GetImportRules(loanID)
	if err != nil {
		return nil, err
	}
	participants, err := s.loanRepo.GetParticipants(loanID)
	if err != nil {
		return nil, err
	}
	valid := make(map[int64]bool, len(participants))
	for _, p := range participants {
		valid[p.ID] = true
	}
	if !valid[forceParticipantID] {
		forceParticipantID = 0 // ignore an invalid override
	}
	// With a single participant there is no one else to pick: every row is theirs.
	if forceParticipantID == 0 && len(participants) == 1 {
		forceParticipantID = participants[0].ID
	}

	result := &ImportResult{Skipped: skippedParse}

	payments := make([]*models.LoanPayment, len(rows))
	for i, row := range rows {
		paymentType := models.PaymentTypeRegular
		if row.Amount < 0 {
			paymentType = models.PaymentTypeWithdrawal
		}

		payment := &models.LoanPayment{
			LoanID:        loanID,
			ParticipantID: placeholderParticipantID,
			Amount:        row.Amount,
			PaymentType:   paymentType,
			PaymentDate:   row.Date,
			Description:   row.Description,
			ImportHash:    row.Hash,
			Source:        models.PaymentSourceImport,
		}
		if forceParticipantID != 0 {
			payment.ParticipantID = forceParticipantID
		} else {
			applyRule(payment, matchRule(row.Description, rules, valid), placeholderParticipantID)
		}
		payments[i] = payment
	}

	// All-or-nothing: rows already imported are skipped by the database;
	// any other failure rolls back the whole file and is returned.
	inserted, err := s.loanRepo.AddImportedPayments(payments)
	if err != nil {
		return nil, fmt.Errorf("import payments for loan %d: %w", loanID, err)
	}
	for i, ok := range inserted {
		if !ok {
			result.Duplicates++
			continue
		}
		result.Imported++
		if payments[i].NeedsAssignment {
			result.Unmatched++
		}
	}

	// Keep the managed accounts in step after a bulk import.
	if err := s.SyncManagedAccounts(loanID); err != nil {
		result.Errors = append(result.Errors, "managed accounts could not be updated")
	}

	return result, nil
}

// applyRule attributes a payment per the matched rule: to its participant,
// to everyone when the rule is shared, or — with no rule — to nobody yet.
func applyRule(payment *models.LoanPayment, rule *models.LoanImportRule, placeholderParticipantID int64) {
	payment.IsShared = false
	payment.NeedsAssignment = false
	switch {
	case rule == nil:
		payment.ParticipantID = placeholderParticipantID
		payment.NeedsAssignment = true
	case rule.IsShared:
		payment.ParticipantID = placeholderParticipantID
		payment.IsShared = true
	default:
		payment.ParticipantID = rule.ParticipantID
	}
}

// ReapplyResult summarises a ReapplyRules run.
type ReapplyResult struct {
	Assigned   int // unassigned rows a rule now attributes
	Unassigned int // rows that had defaulted to the self participant, now waiting for a payer
}

// ReapplyRules runs the payer rules over a loan's imported rows that have
// no deliberate payer yet:
//
//   - unassigned rows a rule now matches are attributed per that rule;
//   - rows that older imports defaulted to the self participant (imported,
//     credited to self, not shared, matching no rule) become unassigned,
//     so they are picked by hand instead of silently counting as the
//     user's money.
//
// Rows already attributed to someone else, shared, or matching a rule are
// left alone so manual corrections are kept.
func (s *LoanService) ReapplyRules(loanID, selfParticipantID int64) (*ReapplyResult, error) {
	rules, err := s.loanRepo.GetImportRules(loanID)
	if err != nil {
		return nil, err
	}
	participants, err := s.loanRepo.GetParticipants(loanID)
	if err != nil {
		return nil, err
	}
	valid := make(map[int64]bool, len(participants))
	for _, p := range participants {
		valid[p.ID] = true
	}
	payments, err := s.loanRepo.GetPayments(loanID)
	if err != nil {
		return nil, err
	}
	// Only a shared loan has anyone else a row could belong to.
	multiplePayers := len(participants) > 1

	result := &ReapplyResult{}
	for _, pay := range payments {
		if pay.Source != models.PaymentSourceImport {
			continue
		}
		rule := matchRule(pay.Description, rules, valid)
		switch {
		case pay.NeedsAssignment && rule != nil:
			if rule.IsShared {
				err = s.loanRepo.SetPaymentShared(pay.ID)
			} else {
				err = s.loanRepo.UpdatePaymentParticipant(pay.ID, rule.ParticipantID)
			}
			if err != nil {
				return nil, err
			}
			result.Assigned++
		case multiplePayers && !pay.NeedsAssignment && !pay.IsShared && rule == nil && pay.ParticipantID == selfParticipantID:
			if err := s.loanRepo.SetPaymentUnassigned(pay.ID); err != nil {
				return nil, err
			}
			result.Unassigned++
		}
	}

	if result.Assigned+result.Unassigned > 0 {
		if err := s.SyncManagedAccounts(loanID); err != nil {
			return result, err
		}
	}
	return result, nil
}

// matchRule returns the first rule whose match text is contained
// (case-insensitively) in the description, or nil. Rules pointing at a
// participant no longer on the loan are ignored.
func matchRule(description string, rules []*models.LoanImportRule, validParticipants map[int64]bool) *models.LoanImportRule {
	desc := strings.ToLower(description)
	for _, rule := range rules {
		needle := strings.ToLower(strings.TrimSpace(rule.MatchText))
		if needle == "" || !strings.Contains(desc, needle) {
			continue
		}
		if rule.IsShared || validParticipants[rule.ParticipantID] {
			return rule
		}
	}
	return nil
}
