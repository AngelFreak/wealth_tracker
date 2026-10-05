package services

import (
	"errors"
	"math"

	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// LoanService holds the settlement and equity math for loans. It is the
// single source of truth for "how much is left", "who owes whom", and
// "how much does this loan add to the user's net worth" — the handlers,
// the detail page, and the dashboard all read these numbers from here so
// they can never disagree.
//
// When a loan has a category, the service also keeps two auto-managed
// accounts in sync with it: a property asset and a loan liability (see
// loan_accounts.go).
type LoanService struct {
	loanRepo        *repository.LoanRepository
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
}

// NewLoanService creates a new LoanService. The account and transaction
// repos are used to maintain the managed accounts for categorised loans;
// they may be nil in contexts that only need the pure settlement math.
func NewLoanService(
	loanRepo *repository.LoanRepository,
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
) *LoanService {
	return &LoanService{
		loanRepo:        loanRepo,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
	}
}

// roundMoney rounds to 2 decimal places to keep float arithmetic tidy in
// the displayed figures.
func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

// ownershipTolerance absorbs float rounding when comparing ownership
// totals against 100% (e.g. 33.33 + 33.33 + 33.34).
const ownershipTolerance = 0.01

// ValidateOwnership returns an error if the participants' ownership
// percentages do not sum to ~100. An empty participant list is allowed
// (a loan with no participants yet contributes nothing).
func ValidateOwnership(participants []*models.LoanParticipant) error {
	if len(participants) == 0 {
		return nil
	}
	total := 0.0
	for _, p := range participants {
		if p.OwnershipPct < 0 {
			return errors.New("ownership percentage cannot be negative")
		}
		total += p.OwnershipPct
	}
	if math.Abs(total-100) > ownershipTolerance {
		return errors.New("ownership percentages must sum to 100")
	}
	return nil
}

// Errors returned by ValidateNewParticipant.
var (
	ErrOwnershipPctInvalid = errors.New("ownership percentage must be greater than 0 and at most 100")
	ErrOwnershipOver100    = errors.New("total ownership would exceed 100%")
)

// ValidOwnershipPct reports whether pct is a usable ownership share:
// a finite number greater than 0 and at most 100.
func ValidOwnershipPct(pct float64) bool {
	return !math.IsNaN(pct) && !math.IsInf(pct, 0) && pct > 0 && pct <= 100
}

// ValidateNewParticipant checks that a participant with the given share
// can be added alongside the existing ones without total ownership
// going over 100%. A total below 100% is allowed, since co-owners are
// added one at a time.
func ValidateNewParticipant(existing []*models.LoanParticipant, pct float64) error {
	if !ValidOwnershipPct(pct) {
		return ErrOwnershipPctInvalid
	}
	total := pct
	for _, p := range existing {
		total += p.OwnershipPct
	}
	if total > 100+ownershipTolerance {
		return ErrOwnershipOver100
	}
	return nil
}

// Summarize computes the full settlement view of a loan: outstanding
// principal, each participant's contributed vs. fair share and resulting
// balance, and the self participant's contribution to net worth.
//
// "Fair share" is each participant's ownership percentage of the amount
// paid down so far. A participant who has contributed more than their
// fair share has a positive Balance and is owed money by the others; a
// participant who has contributed less has a negative Balance and owes
// the others.
func (s *LoanService) Summarize(loanID int64) (*models.LoanSummary, error) {
	loan, err := s.loanRepo.GetByID(loanID)
	if err != nil {
		return nil, err
	}
	if loan == nil {
		return nil, errors.New("loan not found")
	}

	participants, err := s.loanRepo.GetParticipants(loanID)
	if err != nil {
		return nil, err
	}
	payments, err := s.loanRepo.GetPayments(loanID)
	if err != nil {
		return nil, err
	}

	return BuildSummary(loan, participants, payments), nil
}

// BuildSummary is the pure computation behind Summarize, separated so it
// can be unit-tested without a database. It assumes ownership has been
// validated, but is robust to ownership not summing to exactly 100 (it
// uses whatever percentages are present).
func BuildSummary(loan *models.Loan, participants []*models.LoanParticipant, payments []*models.LoanPayment) *models.LoanSummary {
	// netMovement is the signed sum of every row that moves the LOAN
	// balance. For a statement-backed loan (principal 0) the outstanding
	// balance is driven entirely by this: a disbursement is negative,
	// repayments positive, so the running total is negative while in debt.
	//
	// Down payments are EXCLUDED from netMovement: they are equity put
	// into the property, not repayments of the loan, and are already
	// captured by (property value − remaining loan). Counting them in the
	// balance would double-count that equity.
	//
	// totalContributed is the sum of POSITIVE amounts (repayments AND down
	// payments) — the money each person has actually put in. The
	// settlement split ("who owes whom") is based on this, so a larger
	// down payment by one person means the other owes them.
	netMovement := 0.0
	totalContributed := 0.0
	contributed := make(map[int64]float64, len(participants))
	for _, pay := range payments {
		if pay.PaymentType != models.PaymentTypeDownPayment {
			netMovement += pay.Amount
		}
		if pay.Amount <= 0 {
			continue
		}
		totalContributed += pay.Amount
		if pay.IsShared {
			// A shared payment is credited to every participant by their
			// ownership share, so it pays down the balance without tilting
			// the who-owes-whom split.
			for _, p := range participants {
				contributed[p.ID] += (p.OwnershipPct / 100) * pay.Amount
			}
		} else {
			contributed[pay.ParticipantID] += pay.Amount
		}
	}

	// Outstanding = principal minus net movement. With principal 0 and a
	// net-negative statement this is the positive amount still owed. A
	// fully-repaid (or net-positive) loan clamps to 0.
	remaining := loan.Principal - netMovement
	if remaining < 0 {
		remaining = 0
	}

	summary := &models.LoanSummary{
		Loan:         loan,
		TotalPaid:    roundMoney(totalContributed),
		Remaining:    roundMoney(remaining),
		Participants: make([]models.ParticipantSummary, 0, len(participants)),
	}

	for _, p := range participants {
		fairShare := (p.OwnershipPct / 100) * totalContributed
		contrib := contributed[p.ID]
		balance := contrib - fairShare

		ps := models.ParticipantSummary{
			Participant: p,
			Contributed: roundMoney(contrib),
			FairShare:   roundMoney(fairShare),
			Balance:     roundMoney(balance),
		}
		summary.Participants = append(summary.Participants, ps)

		if p.IsSelf {
			share := p.OwnershipPct / 100
			summary.SelfReceivable = roundMoney(balance)
			switch loan.LoanType {
			case models.LoanTypeLent:
				// Money the user lent out: the user's share of what is
				// still outstanding is an asset (others owe it back), and
				// there is no property to count.
				summary.SelfAsset = roundMoney(share * remaining)
				summary.SelfLoanShare = 0
			default:
				// owed / split: the user's share of the property is an
				// asset, and their share of the remaining principal is a
				// liability.
				summary.SelfAsset = roundMoney(share * loan.PropertyValue)
				summary.SelfLoanShare = roundMoney(share * remaining)
			}
		}
	}

	return summary
}

// LoanNetWorth aggregates the net-worth contributions of every active
// loan for a user. Returns the totals the dashboard adds to the
// account-based figures:
//
//	assets      — share of property values + share of money lent out
//	liabilities — share of remaining principal on owed/split loans
//	receivable  — net inter-person balance (positive = others owe the
//	              user, negative = the user owes others)
//
// The user's net contribution is assets - liabilities + receivable.
//
// Loans that have a category set are SKIPPED here: their equity is
// surfaced through managed accounts (a property asset + a loan
// liability) which the dashboard already counts via the account loop.
// Counting them here too would double-count. Only un-categorised loans
// contribute through this path.
func (s *LoanService) LoanNetWorth(userID int64) (assets, liabilities, receivable float64, err error) {
	loans, err := s.loanRepo.GetActiveByUserID(userID)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, loan := range loans {
		if loan.CategoryID != nil {
			continue // surfaced as managed accounts instead
		}
		participants, err := s.loanRepo.GetParticipants(loan.ID)
		if err != nil {
			return 0, 0, 0, err
		}
		payments, err := s.loanRepo.GetPayments(loan.ID)
		if err != nil {
			return 0, 0, 0, err
		}
		summary := BuildSummary(loan, participants, payments)
		assets += summary.SelfAsset
		liabilities += summary.SelfLoanShare
		receivable += summary.SelfReceivable
	}
	return roundMoney(assets), roundMoney(liabilities), roundMoney(receivable), nil
}
