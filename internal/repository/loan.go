package repository

import (
	"database/sql"
	"errors"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// LoanRepository handles loan, participant, and payment database
// operations. Loans the user owes, has lent out, or co-owns (split).
type LoanRepository struct {
	db *database.DB
}

// NewLoanRepository creates a new LoanRepository.
func NewLoanRepository(db *database.DB) *LoanRepository {
	return &LoanRepository{db: db}
}

// --- Loans ---

// Create inserts a new loan and returns its ID.
func (r *LoanRepository) Create(loan *models.Loan) (int64, error) {
	var startDate any
	if loan.StartDate != nil {
		startDate = loan.StartDate.Format("2006-01-02")
	}
	result, err := r.db.Exec(`
		INSERT INTO loans (user_id, name, loan_type, principal, property_value, currency, interest_rate, start_date, is_active, notes, category_id, asset_account_id, liability_account_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, loan.UserID, loan.Name, loan.LoanType, loan.Principal, loan.PropertyValue,
		loan.Currency, loan.InterestRate, startDate, loan.IsActive, loan.Notes,
		loan.CategoryID, loan.AssetAccountID, loan.LiabilityAccountID)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves a loan by ID. Returns (nil, nil) if not found.
func (r *LoanRepository) GetByID(id int64) (*models.Loan, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, name, loan_type, principal, property_value, currency, interest_rate, start_date, is_active, notes, category_id, asset_account_id, liability_account_id, created_at
		FROM loans
		WHERE id = ?
	`, id)
	return scanLoan(row)
}

// GetByUserID retrieves all loans for a user, newest first.
func (r *LoanRepository) GetByUserID(userID int64) ([]*models.Loan, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, loan_type, principal, property_value, currency, interest_rate, start_date, is_active, notes, category_id, asset_account_id, liability_account_id, created_at
		FROM loans
		WHERE user_id = ?
		ORDER BY is_active DESC, created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	loans := make([]*models.Loan, 0)
	for rows.Next() {
		loan, err := scanLoan(rows)
		if err != nil {
			return nil, err
		}
		loans = append(loans, loan)
	}
	return loans, rows.Err()
}

// GetActiveByUserID retrieves only active loans for a user.
func (r *LoanRepository) GetActiveByUserID(userID int64) ([]*models.Loan, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, loan_type, principal, property_value, currency, interest_rate, start_date, is_active, notes, category_id, asset_account_id, liability_account_id, created_at
		FROM loans
		WHERE user_id = ? AND is_active = 1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	loans := make([]*models.Loan, 0)
	for rows.Next() {
		loan, err := scanLoan(rows)
		if err != nil {
			return nil, err
		}
		loans = append(loans, loan)
	}
	return loans, rows.Err()
}

// Update updates an existing loan's editable fields.
func (r *LoanRepository) Update(loan *models.Loan) error {
	var startDate any
	if loan.StartDate != nil {
		startDate = loan.StartDate.Format("2006-01-02")
	}
	result, err := r.db.Exec(`
		UPDATE loans
		SET name = ?, loan_type = ?, principal = ?, property_value = ?, currency = ?, interest_rate = ?, start_date = ?, is_active = ?, notes = ?, category_id = ?, asset_account_id = ?, liability_account_id = ?
		WHERE id = ?
	`, loan.Name, loan.LoanType, loan.Principal, loan.PropertyValue, loan.Currency,
		loan.InterestRate, startDate, loan.IsActive, loan.Notes,
		loan.CategoryID, loan.AssetAccountID, loan.LiabilityAccountID, loan.ID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("loan not found")
	}
	return nil
}

// Delete removes a loan by ID. Participants and payments are removed via
// ON DELETE CASCADE.
func (r *LoanRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM loans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("loan not found")
	}
	return nil
}

// --- Participants ---

// AddParticipant inserts a new participant for a loan and returns its ID.
func (r *LoanRepository) AddParticipant(p *models.LoanParticipant) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO loan_participants (loan_id, name, ownership_pct, is_self)
		VALUES (?, ?, ?, ?)
	`, p.LoanID, p.Name, p.OwnershipPct, p.IsSelf)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetParticipants returns all participants of a loan, self first.
func (r *LoanRepository) GetParticipants(loanID int64) ([]*models.LoanParticipant, error) {
	rows, err := r.db.Query(`
		SELECT id, loan_id, name, ownership_pct, is_self, created_at
		FROM loan_participants
		WHERE loan_id = ?
		ORDER BY is_self DESC, id ASC
	`, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	participants := make([]*models.LoanParticipant, 0)
	for rows.Next() {
		p := &models.LoanParticipant{}
		if err := rows.Scan(&p.ID, &p.LoanID, &p.Name, &p.OwnershipPct, &p.IsSelf, &p.CreatedAt); err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

// GetParticipantByID retrieves a single participant. Returns (nil, nil)
// if not found.
func (r *LoanRepository) GetParticipantByID(id int64) (*models.LoanParticipant, error) {
	p := &models.LoanParticipant{}
	err := r.db.QueryRow(`
		SELECT id, loan_id, name, ownership_pct, is_self, created_at
		FROM loan_participants
		WHERE id = ?
	`, id).Scan(&p.ID, &p.LoanID, &p.Name, &p.OwnershipPct, &p.IsSelf, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// DeleteParticipant removes a participant by ID. Their payments are
// removed via ON DELETE CASCADE.
func (r *LoanRepository) DeleteParticipant(id int64) error {
	result, err := r.db.Exec(`DELETE FROM loan_participants WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("participant not found")
	}
	return nil
}

// --- Payments ---

// AddPayment inserts a new payment for a loan and returns its ID.
func (r *LoanRepository) AddPayment(p *models.LoanPayment) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO loan_payments (loan_id, participant_id, amount, payment_type, payment_date, description)
		VALUES (?, ?, ?, ?, ?, ?)
	`, p.LoanID, p.ParticipantID, p.Amount, p.PaymentType, p.PaymentDate.Format("2006-01-02"), p.Description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetPayments returns all payments of a loan, newest first.
func (r *LoanRepository) GetPayments(loanID int64) ([]*models.LoanPayment, error) {
	rows, err := r.db.Query(`
		SELECT id, loan_id, participant_id, amount, payment_type, payment_date, description, created_at
		FROM loan_payments
		WHERE loan_id = ?
		ORDER BY payment_date DESC, id DESC
	`, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]*models.LoanPayment, 0)
	for rows.Next() {
		p := &models.LoanPayment{}
		var description sql.NullString
		var paymentDate string
		if err := rows.Scan(&p.ID, &p.LoanID, &p.ParticipantID, &p.Amount, &p.PaymentType, &paymentDate, &description, &p.CreatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			p.Description = description.String
		}
		p.PaymentDate = parseDate(paymentDate)
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

// GetPaymentByID retrieves a single payment. Returns (nil, nil) if not
// found.
func (r *LoanRepository) GetPaymentByID(id int64) (*models.LoanPayment, error) {
	p := &models.LoanPayment{}
	var description sql.NullString
	var paymentDate string
	err := r.db.QueryRow(`
		SELECT id, loan_id, participant_id, amount, payment_type, payment_date, description, created_at
		FROM loan_payments
		WHERE id = ?
	`, id).Scan(&p.ID, &p.LoanID, &p.ParticipantID, &p.Amount, &p.PaymentType, &paymentDate, &description, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if description.Valid {
		p.Description = description.String
	}
	p.PaymentDate = parseDate(paymentDate)
	return p, nil
}

// DeletePayment removes a payment by ID.
func (r *LoanRepository) DeletePayment(id int64) error {
	result, err := r.db.Exec(`DELETE FROM loan_payments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("payment not found")
	}
	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows so scanLoan can
// be shared between single-row and multi-row queries.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanLoan reads one loan row, handling the nullable start_date, notes,
// category, and managed-account columns.
func scanLoan(s rowScanner) (*models.Loan, error) {
	loan := &models.Loan{}
	var startDate sql.NullString
	var notes sql.NullString
	var categoryID, assetAccountID, liabilityAccountID sql.NullInt64

	err := s.Scan(
		&loan.ID,
		&loan.UserID,
		&loan.Name,
		&loan.LoanType,
		&loan.Principal,
		&loan.PropertyValue,
		&loan.Currency,
		&loan.InterestRate,
		&startDate,
		&loan.IsActive,
		&notes,
		&categoryID,
		&assetAccountID,
		&liabilityAccountID,
		&loan.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if startDate.Valid && startDate.String != "" {
		d := parseDate(startDate.String)
		loan.StartDate = &d
	}
	if notes.Valid {
		loan.Notes = notes.String
	}
	if categoryID.Valid {
		loan.CategoryID = &categoryID.Int64
	}
	if assetAccountID.Valid {
		loan.AssetAccountID = &assetAccountID.Int64
	}
	if liabilityAccountID.Valid {
		loan.LiabilityAccountID = &liabilityAccountID.Int64
	}
	return loan, nil
}
