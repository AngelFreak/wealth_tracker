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
	Unmatched  int      // rows attributed to the fallback participant
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

		rows = append(rows, ImportedRow{
			Date:        date,
			Amount:      amount,
			Description: description,
			Hash:        importHash(date, amount, description),
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

// ImportPayments writes parsed rows to a loan as payments/withdrawals,
// attributing each to a participant via the loan's rules (falling back
// to fallbackParticipantID), and skipping rows already imported.
//
// rows are typically the output of ParseBankCSV; skippedParse is the
// count it reported so the final summary is complete.
func (s *LoanService) ImportPayments(loanID int64, rows []ImportedRow, skippedParse int, fallbackParticipantID int64) (*ImportResult, error) {
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

	result := &ImportResult{Skipped: skippedParse}

	for _, row := range rows {
		exists, err := s.loanRepo.PaymentHashExists(loanID, row.Hash)
		if err != nil {
			return nil, err
		}
		if exists {
			result.Duplicates++
			continue
		}

		participantID, matched := matchParticipant(row.Description, rules)
		if !matched || !valid[participantID] {
			participantID = fallbackParticipantID
			result.Unmatched++
		}

		paymentType := models.PaymentTypeRegular
		if row.Amount < 0 {
			paymentType = models.PaymentTypeWithdrawal
		}

		if _, err := s.loanRepo.AddPayment(&models.LoanPayment{
			LoanID:        loanID,
			ParticipantID: participantID,
			Amount:        row.Amount,
			PaymentType:   paymentType,
			PaymentDate:   row.Date,
			Description:   row.Description,
			ImportHash:    row.Hash,
			Source:        models.PaymentSourceImport,
		}); err != nil {
			// A unique-index violation means it was imported concurrently;
			// treat as a duplicate rather than failing the whole import.
			result.Duplicates++
			continue
		}
		result.Imported++
	}

	// Keep the managed accounts in step after a bulk import.
	if err := s.SyncManagedAccounts(loanID); err != nil {
		result.Errors = append(result.Errors, "managed accounts could not be updated")
	}

	return result, nil
}

// matchParticipant returns the participant id for the first rule whose
// match text is contained (case-insensitively) in the description.
func matchParticipant(description string, rules []*models.LoanImportRule) (int64, bool) {
	desc := strings.ToLower(description)
	for _, rule := range rules {
		needle := strings.ToLower(strings.TrimSpace(rule.MatchText))
		if needle != "" && strings.Contains(desc, needle) {
			return rule.ParticipantID, true
		}
	}
	return 0, false
}
