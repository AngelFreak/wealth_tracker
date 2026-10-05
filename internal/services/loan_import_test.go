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

func TestParseBankCSV_IdenticalRowsGetDistinctHashes(t *testing.T) {
	csv := strings.Join([]string{
		";Rente af gæld;1;;-100,00;;;31-03-2026;;;;;;;",
		";Rente af gæld;1;;-100,00;;;31-03-2026;;;;;;;",
	}, "\n")
	rows, _, err := ParseBankCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("parsed %d rows, want 2", len(rows))
	}
	if rows[0].Hash == rows[1].Hash {
		t.Error("identical postings got the same hash; the second would be dropped as a duplicate")
	}
	// The first occurrence must keep the original hash so rows imported
	// before this change still dedup on re-import.
	if want := importHash(rows[0].Date, rows[0].Amount, rows[0].Description); rows[0].Hash != want {
		t.Errorf("first occurrence hash changed: got %s, want legacy %s", rows[0].Hash, want)
	}
}

func TestImportPayments_IdenticalRowsBothImportedAndReimportDedups(t *testing.T) {
	db := setupServiceLoanDB(t)
	loanRepo := repository.NewLoanRepository(db)
	svc := NewLoanService(loanRepo, repository.NewAccountRepository(db), repository.NewTransactionRepository(db))
	userID := insertServiceUser(t, db)
	loanID := makeLoan(t, loanRepo, userID, "Statement", models.LoanTypeOwed, 0, 0, true)
	me := addPart(t, loanRepo, loanID, "Me", 100, true)

	csv := strings.Join([]string{
		";Rente af gæld;1;;-100,00;;;31-03-2026;;;;;;;",
		";Rente af gæld;1;;-100,00;;;31-03-2026;;;;;;;",
	}, "\n")

	rows, skipped, _ := ParseBankCSV(strings.NewReader(csv))
	res, err := svc.ImportPayments(loanID, rows, skipped, me, 0)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 2 || res.Duplicates != 0 {
		t.Errorf("import = imported %d / dup %d, want 2/0", res.Imported, res.Duplicates)
	}

	rows2, skipped2, _ := ParseBankCSV(strings.NewReader(csv))
	res2, err := svc.ImportPayments(loanID, rows2, skipped2, me, 0)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res2.Imported != 0 || res2.Duplicates != 2 {
		t.Errorf("re-import = imported %d / dup %d, want 0/2", res2.Imported, res2.Duplicates)
	}
	if got, _ := loanRepo.GetPayments(loanID); len(got) != 2 {
		t.Errorf("stored %d payments, want 2", len(got))
	}
}

func TestImportPayments_DatabaseErrorRollsBackAndIsReturned(t *testing.T) {
	db := setupServiceLoanDB(t)
	loanRepo := repository.NewLoanRepository(db)
	svc := NewLoanService(loanRepo, repository.NewAccountRepository(db), repository.NewTransactionRepository(db))
	userID := insertServiceUser(t, db)
	loanID := makeLoan(t, loanRepo, userID, "Statement", models.LoanTypeOwed, 0, 0, true)
	me := addPart(t, loanRepo, loanID, "Me", 100, true)

	// Force a non-duplicate failure on the second row.
	if _, err := db.Exec(`CREATE TRIGGER fail_boom BEFORE INSERT ON loan_payments
		WHEN NEW.description = 'BOOM' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	csv := strings.Join([]string{
		";Fine row;1;;500,00;;;01-04-2026;;;;;;;",
		";BOOM;1;;600,00;;;02-04-2026;;;;;;;",
		";Another row;1;;700,00;;;03-04-2026;;;;;;;",
	}, "\n")
	rows, skipped, _ := ParseBankCSV(strings.NewReader(csv))

	res, err := svc.ImportPayments(loanID, rows, skipped, me, 0)
	if err == nil {
		t.Fatalf("expected an error, got result %+v (a DB failure was reported as success/duplicate)", res)
	}
	if got, _ := loanRepo.GetPayments(loanID); len(got) != 0 {
		t.Errorf("stored %d payments after a failed import, want 0 (import must be atomic)", len(got))
	}
}
