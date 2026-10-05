package repository

import (
	"path/filepath"
	"testing"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

func setupLoanTestDB(t *testing.T) (*database.DB, int64) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	result, err := db.Exec(`
		INSERT INTO users (email, password_hash, name)
		VALUES (?, ?, ?)
	`, "test@example.com", "hashedpassword", "Test User")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	userID, _ := result.LastInsertId()
	return db, userID
}

func TestLoanRepository_Create_ValidLoan_ReturnsID(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	loan := &models.Loan{
		UserID:        userID,
		Name:          "Apartment Nørrebro",
		LoanType:      models.LoanTypeSplit,
		Principal:     500000,
		PropertyValue: 500000,
		Currency:      "DKK",
		StartDate:     &start,
		IsActive:      true,
	}

	id, err := repo.Create(loan)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestLoanRepository_GetByID_RoundTrip(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	id, err := repo.Create(&models.Loan{
		UserID:        userID,
		Name:          "Apartment",
		LoanType:      models.LoanTypeSplit,
		Principal:     500000,
		PropertyValue: 500000,
		Currency:      "DKK",
		StartDate:     &start,
		IsActive:      true,
		Notes:         "co-owned 50/50",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() returned nil for existing loan")
	}
	if got.Name != "Apartment" || got.Principal != 500000 || got.PropertyValue != 500000 {
		t.Errorf("GetByID() = %+v, fields not preserved", got)
	}
	if got.Notes != "co-owned 50/50" {
		t.Errorf("GetByID() Notes = %q, want %q", got.Notes, "co-owned 50/50")
	}
	if got.StartDate == nil || !got.StartDate.Equal(start) {
		t.Errorf("GetByID() StartDate = %v, want %v", got.StartDate, start)
	}
}

func TestLoanRepository_GetByID_NonExistent_ReturnsNil(t *testing.T) {
	db, _ := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	got, err := repo.GetByID(99999)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if got != nil {
		t.Error("GetByID() should return nil for non-existent ID")
	}
}

func TestLoanRepository_GetByUserID_ReturnsAll(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	for _, name := range []string{"Car loan", "Apartment", "Money to Bob"} {
		if _, err := repo.Create(&models.Loan{
			UserID: userID, Name: name, LoanType: models.LoanTypeOwed,
			Principal: 1000, Currency: "DKK", IsActive: true,
		}); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	loans, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if len(loans) != 3 {
		t.Errorf("GetByUserID() len = %d, want 3", len(loans))
	}
}

func TestLoanRepository_Update(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	id, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Car", LoanType: models.LoanTypeOwed,
		Principal: 100000, Currency: "DKK", IsActive: true,
	})

	loan, _ := repo.GetByID(id)
	loan.Name = "Car (updated)"
	loan.Principal = 90000
	loan.IsActive = false
	if err := repo.Update(loan); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, _ := repo.GetByID(id)
	if got.Name != "Car (updated)" || got.Principal != 90000 || got.IsActive {
		t.Errorf("Update() not applied: %+v", got)
	}
}

func TestLoanRepository_Update_NonExistent_ReturnsError(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	err := repo.Update(&models.Loan{ID: 99999, UserID: userID, Name: "x", LoanType: models.LoanTypeOwed, Currency: "DKK"})
	if err == nil {
		t.Error("Update() should return error for non-existent loan")
	}
}

func TestLoanRepository_Participants_CRUD(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	loanID, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK", IsActive: true,
	})

	selfID, err := repo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 50, IsSelf: true,
	})
	if err != nil {
		t.Fatalf("AddParticipant(self) error = %v", err)
	}
	if _, err := repo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Partner", OwnershipPct: 50, IsSelf: false,
	}); err != nil {
		t.Fatalf("AddParticipant(partner) error = %v", err)
	}

	participants, err := repo.GetParticipants(loanID)
	if err != nil {
		t.Fatalf("GetParticipants() error = %v", err)
	}
	if len(participants) != 2 {
		t.Fatalf("GetParticipants() len = %d, want 2", len(participants))
	}
	// Self should come first (ORDER BY is_self DESC).
	if !participants[0].IsSelf || participants[0].Name != "Me" {
		t.Errorf("GetParticipants() first = %+v, want self 'Me'", participants[0])
	}

	got, err := repo.GetParticipantByID(selfID)
	if err != nil || got == nil {
		t.Fatalf("GetParticipantByID() = %v, %v", got, err)
	}
	if got.OwnershipPct != 50 {
		t.Errorf("GetParticipantByID() OwnershipPct = %v, want 50", got.OwnershipPct)
	}

	if err := repo.DeleteParticipant(selfID); err != nil {
		t.Fatalf("DeleteParticipant() error = %v", err)
	}
	remaining, _ := repo.GetParticipants(loanID)
	if len(remaining) != 1 {
		t.Errorf("after delete, len = %d, want 1", len(remaining))
	}
}

func TestLoanRepository_Payments_CRUD(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	loanID, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK", IsActive: true,
	})
	pid, _ := repo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 50, IsSelf: true,
	})

	payDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	payID, err := repo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: pid, Amount: 250000,
		PaymentType: models.PaymentTypeDownPayment, PaymentDate: payDate,
		Description: "down payment",
	})
	if err != nil {
		t.Fatalf("AddPayment() error = %v", err)
	}

	payments, err := repo.GetPayments(loanID)
	if err != nil {
		t.Fatalf("GetPayments() error = %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("GetPayments() len = %d, want 1", len(payments))
	}
	if payments[0].Amount != 250000 || payments[0].Description != "down payment" {
		t.Errorf("GetPayments()[0] = %+v, fields not preserved", payments[0])
	}
	if !payments[0].PaymentDate.Equal(payDate) {
		t.Errorf("PaymentDate = %v, want %v", payments[0].PaymentDate, payDate)
	}

	got, err := repo.GetPaymentByID(payID)
	if err != nil || got == nil {
		t.Fatalf("GetPaymentByID() = %v, %v", got, err)
	}

	if err := repo.DeletePayment(payID); err != nil {
		t.Fatalf("DeletePayment() error = %v", err)
	}
	after, _ := repo.GetPayments(loanID)
	if len(after) != 0 {
		t.Errorf("after delete, len = %d, want 0", len(after))
	}
}

func TestLoanRepository_Delete_CascadesParticipantsAndPayments(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	loanID, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK", IsActive: true,
	})
	pid, _ := repo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true,
	})
	_, _ = repo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: pid, Amount: 1000,
		PaymentType: models.PaymentTypeRegular, PaymentDate: time.Now(),
	})

	if err := repo.Delete(loanID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if participants, _ := repo.GetParticipants(loanID); len(participants) != 0 {
		t.Errorf("participants not cascade-deleted: %d remain", len(participants))
	}
	if payments, _ := repo.GetPayments(loanID); len(payments) != 0 {
		t.Errorf("payments not cascade-deleted: %d remain", len(payments))
	}
}

func TestLoanRepository_Delete_NonExistent_ReturnsError(t *testing.T) {
	db, _ := setupLoanTestDB(t)
	repo := NewLoanRepository(db)

	if err := repo.Delete(99999); err == nil {
		t.Error("Delete() should return error for non-existent loan")
	}
}

func TestLoanRepository_AddImportedPayments_ReportsInsertedPerRow(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)
	loanID, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Statement", LoanType: models.LoanTypeOwed, Currency: "DKK", IsActive: true,
	})
	pid, _ := repo.AddParticipant(&models.LoanParticipant{LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true})

	pay := func(hash string, amount float64) *models.LoanPayment {
		return &models.LoanPayment{
			LoanID: loanID, ParticipantID: pid, Amount: amount, PaymentType: models.PaymentTypeRegular,
			PaymentDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), ImportHash: hash,
		}
	}

	if _, err := repo.AddImportedPayments([]*models.LoanPayment{pay("a", 100)}); err != nil {
		t.Fatalf("first batch: %v", err)
	}

	// "a" already exists; "b" is new; the second "b" collides within the batch.
	inserted, err := repo.AddImportedPayments([]*models.LoanPayment{pay("a", 100), pay("b", 200), pay("b", 200)})
	if err != nil {
		t.Fatalf("second batch: %v", err)
	}
	want := []bool{false, true, false}
	for i := range want {
		if inserted[i] != want[i] {
			t.Errorf("inserted = %v, want %v", inserted, want)
			break
		}
	}
	if got, _ := repo.GetPayments(loanID); len(got) != 2 {
		t.Errorf("stored %d payments, want 2", len(got))
	}
	for _, p := range mustPayments(t, repo, loanID) {
		if p.Source != models.PaymentSourceImport {
			t.Errorf("payment %q source = %q, want import", p.ImportHash, p.Source)
		}
	}
}

func TestLoanRepository_AddImportedPayments_RejectsMissingHash(t *testing.T) {
	db, userID := setupLoanTestDB(t)
	repo := NewLoanRepository(db)
	loanID, _ := repo.Create(&models.Loan{
		UserID: userID, Name: "Statement", LoanType: models.LoanTypeOwed, Currency: "DKK", IsActive: true,
	})
	pid, _ := repo.AddParticipant(&models.LoanParticipant{LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true})

	_, err := repo.AddImportedPayments([]*models.LoanPayment{
		{LoanID: loanID, ParticipantID: pid, Amount: 100, PaymentType: models.PaymentTypeRegular,
			PaymentDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), ImportHash: "ok"},
		{LoanID: loanID, ParticipantID: pid, Amount: 100, PaymentType: models.PaymentTypeRegular,
			PaymentDate: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	})
	if err == nil {
		t.Fatal("expected an error for a payment without an import hash")
	}
	if got, _ := repo.GetPayments(loanID); len(got) != 0 {
		t.Errorf("stored %d payments, want 0 (batch must roll back)", len(got))
	}
}

func mustPayments(t *testing.T, repo *LoanRepository, loanID int64) []*models.LoanPayment {
	t.Helper()
	ps, err := repo.GetPayments(loanID)
	if err != nil {
		t.Fatalf("GetPayments: %v", err)
	}
	return ps
}
