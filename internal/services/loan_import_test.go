package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

func TestParseDanishAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"4.000,00", 4000, true},
		{"-800,00", -800, true},
		{"1.305,50", 1305.50, true},
		{"-545.957,00", -545957, true},
		{"300,00", 300, true},
		{"-3.976,96", -3976.96, true},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, ok := parseDanishAmount(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseDanishAmount(%q) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseDanishDate(t *testing.T) {
	d, ok := parseDanishDate("02-06-2026")
	if !ok || d.Year() != 2026 || d.Month() != 6 || d.Day() != 2 {
		t.Errorf("parseDanishDate(02-06-2026) = %v,%v", d, ok)
	}
	if _, ok := parseDanishDate("2026-06-02"); ok {
		t.Error("ISO date should not parse as Danish")
	}
	if _, ok := parseDanishDate(""); ok {
		t.Error("empty date should not parse")
	}
}

func TestParseBankCSV_RealFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "posteringer.csv"))
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	rows, skipped, err := ParseBankCSV(f)
	if err != nil {
		t.Fatalf("ParseBankCSV: %v", err)
	}
	// The sample has 24 data rows, all parseable.
	if len(rows) != 24 {
		t.Errorf("parsed %d rows, want 24 (skipped %d)", len(rows), skipped)
	}

	// Spot-check the "Fra Teis" row: 4.000,00 on 02-06-2026.
	var found bool
	for _, r := range rows {
		if r.Description == "Fra Teis" {
			found = true
			if r.Amount != 4000 {
				t.Errorf("'Fra Teis' amount = %v, want 4000", r.Amount)
			}
			if r.Date.Format("2006-01-02") != "2026-06-02" {
				t.Errorf("'Fra Teis' date = %v, want 2026-06-02", r.Date)
			}
		}
	}
	if !found {
		t.Error("'Fra Teis' row not found")
	}

	// A negative posting must come through as a negative amount (withdrawal).
	var sawNegative bool
	for _, r := range rows {
		if r.Amount < 0 {
			sawNegative = true
			break
		}
	}
	if !sawNegative {
		t.Error("expected at least one negative (withdrawal) posting")
	}
}

func TestMatchParticipant(t *testing.T) {
	rules := []*models.LoanImportRule{
		{MatchText: "Fra Teis", ParticipantID: 1},
		{MatchText: "Signe", ParticipantID: 2},
	}
	if id, ok := matchParticipant("Fra Teis", rules); !ok || id != 1 {
		t.Errorf("'Fra Teis' -> %v,%v want 1,true", id, ok)
	}
	// case-insensitive, substring
	if id, ok := matchParticipant("Overførsel fra signe konto", rules); !ok || id != 2 {
		t.Errorf("signe substring -> %v,%v want 2,true", id, ok)
	}
	if _, ok := matchParticipant("Betaling andelslån", rules); ok {
		t.Error("unrelated description should not match")
	}
}

func TestImportPayments_AttributesDedupsAndWithdraws(t *testing.T) {
	db := setupServiceLoanDB(t)
	loanRepo := repository.NewLoanRepository(db)
	svc := NewLoanService(loanRepo, repository.NewAccountRepository(db), repository.NewTransactionRepository(db))
	userID := insertServiceUser(t, db)

	loanID := makeLoan(t, loanRepo, userID, "Apartment", models.LoanTypeSplit, 500000, 500000, true)
	teis := addPart(t, loanRepo, loanID, "Teis", 50, true)
	signe := addPart(t, loanRepo, loanID, "Signe", 50, false)

	// Rule: "Fra Teis" -> Teis.
	if _, err := loanRepo.AddImportRule(&models.LoanImportRule{
		LoanID: loanID, MatchText: "Fra Teis", ParticipantID: teis,
	}); err != nil {
		t.Fatalf("add rule: %v", err)
	}

	csv := strings.Join([]string{
		";Fra Teis;0400 1;0400 2;4.000,00;;;02-06-2026;;;;;;;",   // payment -> Teis (rule)
		";Overførsel;0400 1;0400 2;1.000,00;;;02-02-2026;;;;;;;", // payment, unmatched -> fallback
		";Til primær;0400 2;0400 1;-800,00;;;10-06-2026;;;;;;;",  // withdrawal, unmatched
	}, "\n")

	rows, skipped, err := ParseBankCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 3 || skipped != 0 {
		t.Fatalf("parsed %d rows skipped %d, want 3/0", len(rows), skipped)
	}

	res, err := svc.ImportPayments(loanID, rows, skipped, signe, 0) // fallback = Signe
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 3 {
		t.Errorf("imported %d, want 3", res.Imported)
	}
	if res.Unmatched != 2 {
		t.Errorf("unmatched %d, want 2 (only 'Fra Teis' matched a rule)", res.Unmatched)
	}

	// Verify attribution and the withdrawal sign.
	payments, _ := loanRepo.GetPayments(loanID)
	if len(payments) != 3 {
		t.Fatalf("stored %d payments, want 3", len(payments))
	}
	var teisPaid, withdrawalSeen bool
	for _, p := range payments {
		if p.Description == "Fra Teis" {
			if p.ParticipantID != teis {
				t.Errorf("'Fra Teis' attributed to %d, want Teis(%d)", p.ParticipantID, teis)
			}
			teisPaid = true
		}
		if p.Amount < 0 {
			if p.PaymentType != models.PaymentTypeWithdrawal {
				t.Errorf("negative amount type = %q, want withdrawal", p.PaymentType)
			}
			withdrawalSeen = true
		}
		if p.Source != models.PaymentSourceImport {
			t.Errorf("payment source = %q, want import", p.Source)
		}
	}
	if !teisPaid || !withdrawalSeen {
		t.Errorf("teisPaid=%v withdrawalSeen=%v, want both true", teisPaid, withdrawalSeen)
	}

	// Re-import the SAME rows: everything is a duplicate, nothing added.
	rows2, skipped2, _ := ParseBankCSV(strings.NewReader(csv))
	res2, err := svc.ImportPayments(loanID, rows2, skipped2, signe, 0)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res2.Imported != 0 || res2.Duplicates != 3 {
		t.Errorf("re-import = imported %d / dup %d, want 0/3", res2.Imported, res2.Duplicates)
	}
	if after, _ := loanRepo.GetPayments(loanID); len(after) != 3 {
		t.Errorf("after re-import have %d payments, want 3 (dedup failed)", len(after))
	}
}
