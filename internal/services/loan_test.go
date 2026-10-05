package services

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// participant is a tiny helper to build a participant for the pure-math tests.
func participant(id int64, name string, pct float64, self bool) *models.LoanParticipant {
	return &models.LoanParticipant{ID: id, Name: name, OwnershipPct: pct, IsSelf: self}
}

func payment(participantID int64, amount float64) *models.LoanPayment {
	return &models.LoanPayment{ParticipantID: participantID, Amount: amount}
}

// findParticipant returns the summary row for the named participant.
func findParticipant(t *testing.T, s *models.LoanSummary, name string) models.ParticipantSummary {
	t.Helper()
	for _, ps := range s.Participants {
		if ps.Participant.Name == name {
			return ps
		}
	}
	t.Fatalf("participant %q not found in summary", name)
	return models.ParticipantSummary{}
}

// TestBuildSummary_ApartmentExample encodes the canonical example:
// 500k apartment, 500k loan, A=50%/B=50%, A is self, A pays 250k up
// front and B pays 0. Expect B owes A 125k and A's net worth = 250k.
func TestBuildSummary_ApartmentExample(t *testing.T) {
	loan := &models.Loan{
		Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK",
	}
	a := participant(1, "A", 50, true)  // self
	b := participant(2, "B", 50, false) // co-owner
	payments := []*models.LoanPayment{payment(1, 250000)}

	s := BuildSummary(loan, []*models.LoanParticipant{a, b}, payments)

	if s.TotalPaid != 250000 {
		t.Errorf("TotalPaid = %v, want 250000", s.TotalPaid)
	}
	if s.Remaining != 250000 {
		t.Errorf("Remaining = %v, want 250000", s.Remaining)
	}

	psA := findParticipant(t, s, "A")
	psB := findParticipant(t, s, "B")
	// A contributed 250k, fair share 125k -> overpaid 125k (owed by B).
	if psA.Contributed != 250000 || psA.FairShare != 125000 || psA.Balance != 125000 {
		t.Errorf("A summary = %+v, want contributed 250k / fair 125k / balance +125k", psA)
	}
	// B contributed 0, fair share 125k -> owes 125k.
	if psB.Contributed != 0 || psB.FairShare != 125000 || psB.Balance != -125000 {
		t.Errorf("B summary = %+v, want contributed 0 / fair 125k / balance -125k", psB)
	}

	// Self (A) net worth: asset 250k - loan share 125k + receivable 125k = 250k.
	if s.SelfAsset != 250000 {
		t.Errorf("SelfAsset = %v, want 250000", s.SelfAsset)
	}
	if s.SelfLoanShare != 125000 {
		t.Errorf("SelfLoanShare = %v, want 125000", s.SelfLoanShare)
	}
	if s.SelfReceivable != 125000 {
		t.Errorf("SelfReceivable = %v, want 125000", s.SelfReceivable)
	}
	if got := s.SelfNetWorth(); got != 250000 {
		t.Errorf("SelfNetWorth() = %v, want 250000", got)
	}
}

// TestBuildSummary_EqualContributions: once both have paid their fair
// share, the inter-person balance is zero.
func TestBuildSummary_EqualContributions(t *testing.T) {
	loan := &models.Loan{
		Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK",
	}
	a := participant(1, "A", 50, true)
	b := participant(2, "B", 50, false)
	// A paid 250k up front, then B pays 250k to catch up. Total paid 500k.
	payments := []*models.LoanPayment{payment(1, 250000), payment(2, 250000)}

	s := BuildSummary(loan, []*models.LoanParticipant{a, b}, payments)

	if s.Remaining != 0 {
		t.Errorf("Remaining = %v, want 0", s.Remaining)
	}
	psA := findParticipant(t, s, "A")
	psB := findParticipant(t, s, "B")
	if psA.Balance != 0 || psB.Balance != 0 {
		t.Errorf("balances A=%v B=%v, want both 0", psA.Balance, psB.Balance)
	}
	// Loan fully paid: A's net worth = 250k asset - 0 - 0.
	if got := s.SelfNetWorth(); got != 250000 {
		t.Errorf("SelfNetWorth() = %v, want 250000", got)
	}
}

// TestBuildSummary_SimpleOwedLoan: a single participant at 100% (a car
// loan). No property, so the loan is purely a liability.
func TestBuildSummary_SimpleOwedLoan(t *testing.T) {
	loan := &models.Loan{
		Name: "Car loan", LoanType: models.LoanTypeOwed,
		Principal: 100000, PropertyValue: 0, Currency: "DKK",
	}
	me := participant(1, "Me", 100, true)
	payments := []*models.LoanPayment{payment(1, 30000)}

	s := BuildSummary(loan, []*models.LoanParticipant{me}, payments)

	if s.Remaining != 70000 {
		t.Errorf("Remaining = %v, want 70000", s.Remaining)
	}
	// Only participant, so no inter-person balance.
	psMe := findParticipant(t, s, "Me")
	if psMe.Balance != 0 {
		t.Errorf("Me balance = %v, want 0", psMe.Balance)
	}
	// Net worth: 0 asset - 70k liability = -70k.
	if s.SelfAsset != 0 || s.SelfLoanShare != 70000 {
		t.Errorf("self asset=%v loanShare=%v, want 0 / 70000", s.SelfAsset, s.SelfLoanShare)
	}
	if got := s.SelfNetWorth(); got != -70000 {
		t.Errorf("SelfNetWorth() = %v, want -70000", got)
	}
}

// TestBuildSummary_SimpleOwnedAsset: a 100% mortgage on a property worth
// more than the remaining loan yields positive equity.
func TestBuildSummary_SimpleOwnedAsset(t *testing.T) {
	loan := &models.Loan{
		Name: "House", LoanType: models.LoanTypeOwed,
		Principal: 1000000, PropertyValue: 1500000, Currency: "DKK",
	}
	me := participant(1, "Me", 100, true)
	payments := []*models.LoanPayment{payment(1, 200000)}

	s := BuildSummary(loan, []*models.LoanParticipant{me}, payments)

	// Remaining 800k. Net worth = 1.5M asset - 800k = 700k.
	if s.Remaining != 800000 {
		t.Errorf("Remaining = %v, want 800000", s.Remaining)
	}
	if got := s.SelfNetWorth(); got != 700000 {
		t.Errorf("SelfNetWorth() = %v, want 700000", got)
	}
}

// TestBuildSummary_StatementBackedLoan models an account-statement loan
// (principal 0): the outstanding balance is the magnitude of the signed
// running total, and the disbursement/fees don't scramble the split.
func TestBuildSummary_StatementBackedLoan(t *testing.T) {
	loan := &models.Loan{
		Name: "Andelslån", LoanType: models.LoanTypeSplit,
		Principal: 0, PropertyValue: 1195000, Currency: "DKK",
	}
	teis := participant(1, "Teis", 50, true)
	signe := participant(2, "Signe", 50, false)

	// Disbursement (negative), then repayments. Net = -100,000 → owed 100,000.
	payments := []*models.LoanPayment{
		{ParticipantID: 1, Amount: -150000}, // disbursement (no person "contributed" this)
		{ParticipantID: 1, Amount: 30000},   // Teis repays
		{ParticipantID: 2, Amount: 20000},   // Signe repays
	}

	s := BuildSummary(loan, []*models.LoanParticipant{teis, signe}, payments)

	// netMovement = -150000+30000+20000 = -100000 → remaining 100000.
	if s.Remaining != 100000 {
		t.Errorf("Remaining = %v, want 100000", s.Remaining)
	}
	// Contributions = positive only: total 50000; fair share each 25000.
	if s.TotalPaid != 50000 {
		t.Errorf("TotalPaid (positive contributions) = %v, want 50000", s.TotalPaid)
	}
	psT := findParticipant(t, s, "Teis")
	psS := findParticipant(t, s, "Signe")
	// Teis contributed 30k vs fair 25k → owed 5k; Signe 20k vs 25k → owes 5k.
	if psT.Balance != 5000 {
		t.Errorf("Teis balance = %v, want +5000", psT.Balance)
	}
	if psS.Balance != -5000 {
		t.Errorf("Signe balance = %v, want -5000", psS.Balance)
	}
	// Self equity: 50% property (597500) - 50% remaining (50000) + receivable 5000.
	if s.SelfAsset != 597500 || s.SelfLoanShare != 50000 || s.SelfReceivable != 5000 {
		t.Errorf("self asset=%v loan=%v recv=%v, want 597500/50000/5000", s.SelfAsset, s.SelfLoanShare, s.SelfReceivable)
	}
}

// TestBuildSummary_DownPaymentIsEquityNotBalance: a down payment is the
// user's equity into the property, not a repayment of the loan, so it
// counts toward the split but NOT toward the loan balance.
func TestBuildSummary_DownPaymentIsEquityNotBalance(t *testing.T) {
	loan := &models.Loan{
		Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 0, PropertyValue: 1195000, Currency: "DKK",
	}
	teis := participant(1, "Teis", 50, true)
	signe := participant(2, "Signe", 50, false)

	payments := []*models.LoanPayment{
		// The statement: disbursement + repayments netting to -100,000.
		{ParticipantID: 1, Amount: -150000, PaymentType: models.PaymentTypeRegular},
		{ParticipantID: 1, Amount: 50000, PaymentType: models.PaymentTypeRegular},
		// A down payment of 665,209 by Teis — equity, must NOT cut the balance.
		{ParticipantID: 1, Amount: 665209, PaymentType: models.PaymentTypeDownPayment},
	}

	s := BuildSummary(loan, []*models.LoanParticipant{teis, signe}, payments)

	// Balance excludes the down payment: net of the two regular rows is
	// -100,000 → owed 100,000. The 665,209 must not reduce it.
	if s.Remaining != 100000 {
		t.Errorf("Remaining = %v, want 100000 (down payment must not cut the balance)", s.Remaining)
	}
	// The down payment DOES count toward contributions/split: Teis put in
	// 50,000 + 665,209 = 715,209; fair share = 357,604.50 → owed by Signe.
	psT := findParticipant(t, s, "Teis")
	if psT.Contributed != 715209 {
		t.Errorf("Teis contributed = %v, want 715209 (incl. down payment)", psT.Contributed)
	}
}

// TestBuildSummary_SharedPayment: a payment marked shared is credited to
// each participant by ownership %, so it pays down the balance without
// shifting the who-owes-whom split.
func TestBuildSummary_SharedPayment(t *testing.T) {
	loan := &models.Loan{
		Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK",
	}
	a := participant(1, "A", 50, true)
	b := participant(2, "B", 50, false)

	// A pays 100k (their own), then a 200k SHARED payment (50/50).
	payments := []*models.LoanPayment{
		{ParticipantID: 1, Amount: 100000},
		{ParticipantID: 1, Amount: 200000, IsShared: true}, // participant_id ignored
	}

	s := BuildSummary(loan, []*models.LoanParticipant{a, b}, payments)

	// Total contributed 300k; remaining 200k.
	if s.Remaining != 200000 {
		t.Errorf("Remaining = %v, want 200000", s.Remaining)
	}
	// Contributions: A = 100k + 100k(shared half) = 200k; B = 100k(shared half).
	// Fair share each = 150k. A overpaid 50k → B owes A 50k.
	psA := findParticipant(t, s, "A")
	psB := findParticipant(t, s, "B")
	if psA.Contributed != 200000 || psA.Balance != 50000 {
		t.Errorf("A = %+v, want contributed 200k / balance +50k", psA)
	}
	if psB.Contributed != 100000 || psB.Balance != -50000 {
		t.Errorf("B = %+v, want contributed 100k / balance -50k", psB)
	}
}

// TestBuildSummary_LentLoan: money the user lent to a friend. The
// outstanding amount is an asset (a receivable), not a liability.
func TestBuildSummary_LentLoan(t *testing.T) {
	loan := &models.Loan{
		Name: "Loan to Bob", LoanType: models.LoanTypeLent,
		Principal: 50000, PropertyValue: 0, Currency: "DKK",
	}
	me := participant(1, "Me", 100, true)
	// Bob has repaid 20k so far.
	payments := []*models.LoanPayment{payment(1, 20000)}

	s := BuildSummary(loan, []*models.LoanParticipant{me}, payments)

	// Remaining 30k is still owed to the user -> asset.
	if s.Remaining != 30000 {
		t.Errorf("Remaining = %v, want 30000", s.Remaining)
	}
	if s.SelfAsset != 30000 || s.SelfLoanShare != 0 {
		t.Errorf("lent: asset=%v loanShare=%v, want 30000 / 0", s.SelfAsset, s.SelfLoanShare)
	}
	if got := s.SelfNetWorth(); got != 30000 {
		t.Errorf("SelfNetWorth() = %v, want 30000", got)
	}
}

func TestValidateOwnership(t *testing.T) {
	tests := []struct {
		name    string
		pcts    []float64
		wantErr bool
	}{
		{"sums to 100", []float64{50, 50}, false},
		{"single 100", []float64{100}, false},
		{"thirds", []float64{33.34, 33.33, 33.33}, false},
		{"empty allowed", nil, false},
		{"too low", []float64{50, 40}, true},
		{"too high", []float64{60, 50}, true},
		{"negative", []float64{-10, 110}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ps []*models.LoanParticipant
			for i, pct := range tt.pcts {
				ps = append(ps, participant(int64(i+1), "p", pct, i == 0))
			}
			err := ValidateOwnership(ps)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOwnership(%v) err = %v, wantErr = %v", tt.pcts, err, tt.wantErr)
			}
		})
	}
}

// TestLoanNetWorth_AggregatesAcrossLoans exercises the DB-backed path:
// two active loans plus one inactive loan that must be excluded.
func TestLoanNetWorth_AggregatesAcrossLoans(t *testing.T) {
	db := setupServiceLoanDB(t)
	loanRepo := repository.NewLoanRepository(db)
	svc := NewLoanService(loanRepo, repository.NewAccountRepository(db), repository.NewTransactionRepository(db))
	userID := insertServiceUser(t, db)

	// Loan 1: the apartment example -> self net worth 250000
	//         (asset 250k, liability 125k, receivable +125k).
	l1 := makeLoan(t, loanRepo, userID, "Apartment", models.LoanTypeSplit, 500000, 500000, true)
	self1 := addPart(t, loanRepo, l1, "A", 50, true)
	addPart(t, loanRepo, l1, "B", 50, false)
	addPay(t, loanRepo, l1, self1, 250000)

	// Loan 2: simple car loan, remaining 70k -> liability 70k.
	l2 := makeLoan(t, loanRepo, userID, "Car", models.LoanTypeOwed, 100000, 0, true)
	self2 := addPart(t, loanRepo, l2, "Me", 100, true)
	addPay(t, loanRepo, l2, self2, 30000)

	// Loan 3: inactive, must be ignored.
	l3 := makeLoan(t, loanRepo, userID, "Old", models.LoanTypeOwed, 999999, 0, false)
	self3 := addPart(t, loanRepo, l3, "Me", 100, true)
	addPay(t, loanRepo, l3, self3, 0)

	assets, liabilities, receivable, err := svc.LoanNetWorth(userID)
	if err != nil {
		t.Fatalf("LoanNetWorth() error = %v", err)
	}
	// assets: 250k (apartment) + 0 (car) = 250k
	// liabilities: 125k (apartment) + 70k (car) = 195k
	// receivable: +125k (apartment) + 0 (car) = 125k
	if assets != 250000 {
		t.Errorf("assets = %v, want 250000", assets)
	}
	if liabilities != 195000 {
		t.Errorf("liabilities = %v, want 195000", liabilities)
	}
	if receivable != 125000 {
		t.Errorf("receivable = %v, want 125000", receivable)
	}
	// Net contribution = 250000 - 195000 + 125000 = 180000.
	if net := assets - liabilities + receivable; net != 180000 {
		t.Errorf("net = %v, want 180000", net)
	}
}

// --- DB helpers for the service-layer test ---

func setupServiceLoanDB(t *testing.T) *database.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertServiceUser(t *testing.T, db *database.DB) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, password_hash, name) VALUES (?, ?, ?)`,
		"svc@example.com", "x", "Svc User")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func makeLoan(t *testing.T, r *repository.LoanRepository, userID int64, name, typ string, principal, propValue float64, active bool) int64 {
	t.Helper()
	id, err := r.Create(&models.Loan{
		UserID: userID, Name: name, LoanType: typ,
		Principal: principal, PropertyValue: propValue, Currency: "DKK", IsActive: active,
	})
	if err != nil {
		t.Fatalf("create loan %q: %v", name, err)
	}
	return id
}

func addPart(t *testing.T, r *repository.LoanRepository, loanID int64, name string, pct float64, self bool) int64 {
	t.Helper()
	id, err := r.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: name, OwnershipPct: pct, IsSelf: self,
	})
	if err != nil {
		t.Fatalf("add participant %q: %v", name, err)
	}
	return id
}

func addPay(t *testing.T, r *repository.LoanRepository, loanID, participantID int64, amount float64) {
	t.Helper()
	if _, err := r.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: participantID, Amount: amount,
		PaymentType: models.PaymentTypeRegular, PaymentDate: time.Now(),
	}); err != nil {
		t.Fatalf("add payment: %v", err)
	}
}

func TestValidateNewParticipant(t *testing.T) {
	tests := []struct {
		name     string
		existing []float64
		pct      float64
		wantErr  error
	}{
		{"fills to 100", []float64{50}, 50, nil},
		{"stays under 100", []float64{30}, 20, nil},
		{"thirds rounding", []float64{33.33, 33.33}, 33.34, nil},
		{"pushes over 100", []float64{50}, 60, ErrOwnershipOver100},
		{"zero", []float64{50}, 0, ErrOwnershipPctInvalid},
		{"negative", []float64{50}, -5, ErrOwnershipPctInvalid},
		{"over 100 alone", nil, 101, ErrOwnershipPctInvalid},
		{"NaN", []float64{50}, math.NaN(), ErrOwnershipPctInvalid},
		{"Inf", []float64{50}, math.Inf(1), ErrOwnershipPctInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ps []*models.LoanParticipant
			for i, pct := range tt.existing {
				ps = append(ps, participant(int64(i+1), "p", pct, i == 0))
			}
			if err := ValidateNewParticipant(ps, tt.pct); err != tt.wantErr {
				t.Errorf("ValidateNewParticipant(%v, %v) = %v, want %v", tt.existing, tt.pct, err, tt.wantErr)
			}
		})
	}
}
