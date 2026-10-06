package handlers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
	"wealth_tracker/internal/services"
)

func setupLoanHandlerTest(t *testing.T) (*LoanHandler, *repository.LoanRepository, *models.User, *models.User) {
	t.Helper()
	handler, loanRepo, owner, other, _ := setupLoanHandlerTestDB(t)
	return handler, loanRepo, owner, other
}

// setupLoanHandlerTestDB is setupLoanHandlerTest that also returns the
// database, for tests that need to manipulate it directly.
func setupLoanHandlerTestDB(t *testing.T) (*LoanHandler, *repository.LoanRepository, *models.User, *models.User, *database.DB) {
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

	loanRepo := repository.NewLoanRepository(db)
	loanService := services.NewLoanService(loanRepo, repository.NewAccountRepository(db), repository.NewTransactionRepository(db))
	handler := NewLoanHandler(nil, loanRepo, loanService, repository.NewCategoryRepository(db))

	owner := insertHandlerUser(t, db, "owner@example.com")
	other := insertHandlerUser(t, db, "other@example.com")
	return handler, loanRepo, owner, other, db
}

func insertHandlerUser(t *testing.T, db *database.DB, email string) *models.User {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, password_hash, name) VALUES (?, ?, ?)`, email, "x", "User")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return &models.User{ID: id, Email: email, Name: "User"}
}

// authedRequest builds a POST request with form values, the user in
// context, and chi URL params populated.
func authedRequest(user *models.User, target string, form url.Values, params map[string]string) *http.Request {
	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, user))

	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestLoanHandler_Create_CreatesLoanAndSelfParticipant(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	form := url.Values{
		"name":               {"Apartment"},
		"loan_type":          {models.LoanTypeSplit},
		"principal":          {"500000"},
		"property_value":     {"500000"},
		"currency":           {"DKK"},
		"self_name":          {"Me"},
		"self_ownership_pct": {"50"},
	}
	req := authedRequest(owner, "/loans", form, nil)
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create() status = %d, want 303", rec.Code)
	}

	loans, _ := loanRepo.GetByUserID(owner.ID)
	if len(loans) != 1 {
		t.Fatalf("expected 1 loan, got %d", len(loans))
	}
	loan := loans[0]
	if loan.Name != "Apartment" || loan.Principal != 500000 || loan.PropertyValue != 500000 {
		t.Errorf("loan fields not saved: %+v", loan)
	}

	participants, _ := loanRepo.GetParticipants(loan.ID)
	if len(participants) != 1 {
		t.Fatalf("expected 1 self participant, got %d", len(participants))
	}
	if !participants[0].IsSelf || participants[0].OwnershipPct != 50 || participants[0].Name != "Me" {
		t.Errorf("self participant wrong: %+v", participants[0])
	}

	// Redirect should point at the new loan's detail page.
	if loc := rec.Header().Get("Location"); loc != "/loans/"+strconv.FormatInt(loan.ID, 10) {
		t.Errorf("redirect = %q, want /loans/%d", loc, loan.ID)
	}
}

func TestLoanHandler_RecordPayment_HappyPath(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK", IsActive: true,
	})
	selfID, _ := loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 50, IsSelf: true,
	})
	loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Partner", OwnershipPct: 50, IsSelf: false,
	})

	form := url.Values{
		"participant_id": {strconv.FormatInt(selfID, 10)},
		"amount":         {"250000"},
		"payment_type":   {models.PaymentTypeRegular},
		"payment_date":   {"2024-02-01"},
		"description":    {"repayment"},
	}
	req := authedRequest(owner, "/loans/"+strconv.FormatInt(loanID, 10)+"/payments", form,
		map[string]string{"id": strconv.FormatInt(loanID, 10)})
	rec := httptest.NewRecorder()

	handler.RecordPayment(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("RecordPayment() status = %d, want 303", rec.Code)
	}
	payments, _ := loanRepo.GetPayments(loanID)
	if len(payments) != 1 || payments[0].Amount != 250000 {
		t.Fatalf("payment not recorded: %+v", payments)
	}

	// Verify the settlement math now reports B owes A 125k.
	summary, err := handler.loanService.Summarize(loanID)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if summary.Remaining != 250000 {
		t.Errorf("Remaining = %v, want 250000", summary.Remaining)
	}
	var selfBalance, partnerBalance float64
	for _, ps := range summary.Participants {
		if ps.Participant.IsSelf {
			selfBalance = ps.Balance
		} else {
			partnerBalance = ps.Balance
		}
	}
	if selfBalance != 125000 {
		t.Errorf("self balance = %v, want +125000", selfBalance)
	}
	if partnerBalance != -125000 {
		t.Errorf("partner balance = %v, want -125000", partnerBalance)
	}
}

func TestLoanHandler_RecordPayment_RejectsZeroAmount(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Car", LoanType: models.LoanTypeOwed,
		Principal: 100000, Currency: "DKK", IsActive: true,
	})
	selfID, _ := loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true,
	})

	form := url.Values{
		"participant_id": {strconv.FormatInt(selfID, 10)},
		"amount":         {"0"},
	}
	req := authedRequest(owner, "/loans/"+strconv.FormatInt(loanID, 10)+"/payments", form,
		map[string]string{"id": strconv.FormatInt(loanID, 10)})
	rec := httptest.NewRecorder()

	handler.RecordPayment(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("RecordPayment(0) status = %d, want 400", rec.Code)
	}
	if payments, _ := loanRepo.GetPayments(loanID); len(payments) != 0 {
		t.Errorf("zero-amount payment should not be recorded, got %d", len(payments))
	}
}

func TestLoanHandler_Detail_OtherUsersLoan_Forbidden(t *testing.T) {
	handler, loanRepo, owner, other := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, PropertyValue: 500000, Currency: "DKK", IsActive: true,
	})

	// "other" tries to view owner's loan.
	req := httptest.NewRequest("GET", "/loans/"+strconv.FormatInt(loanID, 10), nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, other))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(loanID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.Detail(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Detail() for other user's loan status = %d, want 403", rec.Code)
	}
}

func TestLoanHandler_Delete_OtherUsersLoan_Forbidden(t *testing.T) {
	handler, loanRepo, owner, other := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, Currency: "DKK", IsActive: true,
	})

	req := authedRequest(other, "/loans/"+strconv.FormatInt(loanID, 10)+"/delete", url.Values{},
		map[string]string{"id": strconv.FormatInt(loanID, 10)})
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Delete() for other user's loan status = %d, want 403", rec.Code)
	}
	// Loan must still exist.
	if loan, _ := loanRepo.GetByID(loanID); loan == nil {
		t.Error("loan was deleted despite forbidden")
	}
}

func TestLoanHandler_DeleteParticipant_CannotRemoveSelf(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, Currency: "DKK", IsActive: true,
	})
	selfID, _ := loanRepo.AddParticipant(&models.LoanParticipant{
		LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true,
	})

	req := authedRequest(owner, "/loans/x/participants/x/delete", url.Values{}, map[string]string{
		"id":            strconv.FormatInt(loanID, 10),
		"participantID": strconv.FormatInt(selfID, 10),
	})
	rec := httptest.NewRecorder()

	handler.DeleteParticipant(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DeleteParticipant(self) status = %d, want 400", rec.Code)
	}
	if ps, _ := loanRepo.GetParticipants(loanID); len(ps) != 1 {
		t.Errorf("self participant should remain, got %d participants", len(ps))
	}
}

// buildMultipartCSV builds a multipart request body with a CSV file field.
func buildMultipartCSV(t *testing.T, csv string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, csv); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	mw.Close()
	return &body, mw.FormDataContentType()
}

func TestLoanHandler_ImportCSV_CreatesPaymentsAndDedups(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 500000, Currency: "DKK", IsActive: true,
	})
	loanRepo.AddParticipant(&models.LoanParticipant{LoanID: loanID, Name: "Me", OwnershipPct: 100, IsSelf: true})

	csv := ";Fra Teis;0400 1;0400 2;4.000,00;;;02-06-2026;;;;;;;\n;Til primær;0400 2;0400 1;-800,00;;;10-06-2026;;;;;;;\n"

	doImport := func() *httptest.ResponseRecorder {
		body, ctype := buildMultipartCSV(t, csv)
		req := httptest.NewRequest("POST", "/loans/"+strconv.FormatInt(loanID, 10)+"/import", body)
		req.Header.Set("Content-Type", ctype)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, owner))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", strconv.FormatInt(loanID, 10))
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		handler.ImportCSV(rec, req)
		return rec
	}

	rec := doImport()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ImportCSV status = %d, want 303", rec.Code)
	}
	payments, _ := loanRepo.GetPayments(loanID)
	if len(payments) != 2 {
		t.Fatalf("after import have %d payments, want 2", len(payments))
	}
	// One of them must be a withdrawal (negative).
	var withdrawals int
	for _, p := range payments {
		if p.Amount < 0 {
			withdrawals++
		}
	}
	if withdrawals != 1 {
		t.Errorf("withdrawals = %d, want 1", withdrawals)
	}

	// Re-import the same file: dedup, no new payments.
	doImport()
	if after, _ := loanRepo.GetPayments(loanID); len(after) != 2 {
		t.Errorf("after re-import have %d payments, want 2 (dedup)", len(after))
	}
}

func TestLoanHandler_UpdatePaymentPayer_Reassigns(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 0, PropertyValue: 1000000, Currency: "DKK", IsActive: true,
	})
	me := mustAddPart(t, loanRepo, loanID, "Me", 50, true)
	partner := mustAddPart(t, loanRepo, loanID, "Partner", 50, false)
	payID, _ := loanRepo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: me, Amount: 4000,
		PaymentType: models.PaymentTypeRegular, PaymentDate: time.Now(),
	})

	form := url.Values{"participant_id": {strconv.FormatInt(partner, 10)}}
	req := authedRequest(owner, "/loans/x/payments/x/payer", form, map[string]string{
		"id":        strconv.FormatInt(loanID, 10),
		"paymentID": strconv.FormatInt(payID, 10),
	})
	rec := httptest.NewRecorder()
	handler.UpdatePaymentPayer(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("UpdatePaymentPayer status = %d, want 303", rec.Code)
	}
	got, _ := loanRepo.GetPaymentByID(payID)
	if got.ParticipantID != partner {
		t.Errorf("payment reassigned to %d, want Partner(%d)", got.ParticipantID, partner)
	}
}

func TestLoanHandler_UpdatePaymentPayer_AssignsUnassignedRow(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 0, PropertyValue: 1000000, Currency: "DKK", IsActive: true,
	})
	me := mustAddPart(t, loanRepo, loanID, "Me", 50, true)
	partner := mustAddPart(t, loanRepo, loanID, "Partner", 50, false)
	payID, _ := loanRepo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: me, Amount: 4000, NeedsAssignment: true,
		PaymentType: models.PaymentTypeRegular, PaymentDate: time.Now(),
	})

	post := func(value string) *models.LoanPayment {
		t.Helper()
		req := authedRequest(owner, "/loans/x/payments/x/payer", url.Values{"participant_id": {value}}, map[string]string{
			"id":        strconv.FormatInt(loanID, 10),
			"paymentID": strconv.FormatInt(payID, 10),
		})
		rec := httptest.NewRecorder()
		handler.UpdatePaymentPayer(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("UpdatePaymentPayer(%s) status = %d, want 303", value, rec.Code)
		}
		got, _ := loanRepo.GetPaymentByID(payID)
		return got
	}

	if got := post(strconv.FormatInt(partner, 10)); got.NeedsAssignment || got.ParticipantID != partner {
		t.Errorf("after picking Partner: participant %d unassigned=%v", got.ParticipantID, got.NeedsAssignment)
	}
	if got := post("unassigned"); !got.NeedsAssignment || got.IsShared {
		t.Errorf("after unassigning: unassigned=%v shared=%v", got.NeedsAssignment, got.IsShared)
	}
	if got := post("shared"); got.NeedsAssignment || !got.IsShared {
		t.Errorf("after sharing: unassigned=%v shared=%v", got.NeedsAssignment, got.IsShared)
	}
}

func TestLoanHandler_AddRule_SharedAndReapply(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit,
		Principal: 0, PropertyValue: 1000000, Currency: "DKK", IsActive: true,
	})
	me := mustAddPart(t, loanRepo, loanID, "Me", 50, true)
	mustAddPart(t, loanRepo, loanID, "Partner", 50, false)
	params := map[string]string{"id": strconv.FormatInt(loanID, 10)}

	req := authedRequest(owner, "/loans/x/rules", url.Values{"match_text": {"Rente"}, "participant_id": {"shared"}}, params)
	rec := httptest.NewRecorder()
	handler.AddRule(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("AddRule status = %d, want 303", rec.Code)
	}
	rules, _ := loanRepo.GetImportRules(loanID)
	if len(rules) != 1 || !rules[0].IsShared {
		t.Fatalf("rules = %+v, want one shared rule", rules)
	}

	interest, _ := loanRepo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: me, Amount: -100, NeedsAssignment: true, Source: models.PaymentSourceImport,
		PaymentType: models.PaymentTypeWithdrawal, PaymentDate: time.Now(), Description: "Rente af gæld", ImportHash: "a",
	})
	legacy, _ := loanRepo.AddPayment(&models.LoanPayment{
		LoanID: loanID, ParticipantID: me, Amount: 168000, Source: models.PaymentSourceImport,
		PaymentType: models.PaymentTypeRegular, PaymentDate: time.Now(), Description: "Overførsel", ImportHash: "b",
	})

	req = authedRequest(owner, "/loans/x/rules/apply", url.Values{}, params)
	rec = httptest.NewRecorder()
	handler.ReapplyRules(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ReapplyRules status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "assigned=1") || !strings.Contains(loc, "unassigned=1") {
		t.Errorf("redirect = %q, want assigned=1 and unassigned=1", loc)
	}
	if got, _ := loanRepo.GetPaymentByID(interest); !got.IsShared || got.NeedsAssignment {
		t.Errorf("interest row: shared=%v unassigned=%v, want shared", got.IsShared, got.NeedsAssignment)
	}
	if got, _ := loanRepo.GetPaymentByID(legacy); !got.NeedsAssignment {
		t.Error("legacy row defaulted to self should be unassigned")
	}
}

// mustAddPart adds a participant and returns its id (test helper).
func mustAddPart(t *testing.T, r *repository.LoanRepository, loanID int64, name string, pct float64, self bool) int64 {
	t.Helper()
	id, err := r.AddParticipant(&models.LoanParticipant{LoanID: loanID, Name: name, OwnershipPct: pct, IsSelf: self})
	if err != nil {
		t.Fatalf("add participant: %v", err)
	}
	return id
}

func TestLoanHandler_ImportCSV_DatabaseErrorRedirectsWithFailAndStoresNothing(t *testing.T) {
	handler, loanRepo, owner, _, db := setupLoanHandlerTestDB(t)

	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Statement", LoanType: models.LoanTypeOwed, Currency: "DKK", IsActive: true,
	})
	mustAddPart(t, loanRepo, loanID, "Me", 100, true)

	if _, err := db.Exec(`CREATE TRIGGER fail_boom BEFORE INSERT ON loan_payments
		WHEN NEW.description = 'BOOM' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	csv := ";Fine row;1;;500,00;;;01-04-2026;;;;;;;\n;BOOM;1;;600,00;;;02-04-2026;;;;;;;\n"
	body, ctype := buildMultipartCSV(t, csv)
	req := httptest.NewRequest("POST", "/loans/"+strconv.FormatInt(loanID, 10)+"/import", body)
	req.Header.Set("Content-Type", ctype)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, owner))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(loanID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	handler.ImportCSV(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "import=fail") {
		t.Errorf("Location = %q, want it to report import=fail", loc)
	}
	if got, _ := loanRepo.GetPayments(loanID); len(got) != 0 {
		t.Errorf("stored %d payments after a failed import, want 0", len(got))
	}
}

func addParticipantRequest(t *testing.T, handler *LoanHandler, owner *models.User, loanID int64, pct string) *httptest.ResponseRecorder {
	t.Helper()
	id := strconv.FormatInt(loanID, 10)
	req := authedRequest(owner, "/loans/"+id+"/participants",
		url.Values{"name": {"Partner"}, "ownership_pct": {pct}}, map[string]string{"id": id})
	rec := httptest.NewRecorder()
	handler.AddParticipant(rec, req)
	return rec
}

func TestLoanHandler_AddParticipant_RejectsOwnershipOver100(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)
	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit, Currency: "DKK", IsActive: true,
	})
	mustAddPart(t, loanRepo, loanID, "Me", 50, true)

	rec := addParticipantRequest(t, handler, owner, loanID, "60")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 back to the loan", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "participant=over100") {
		t.Errorf("Location = %q, want participant=over100", loc)
	}
	if ps, _ := loanRepo.GetParticipants(loanID); len(ps) != 1 {
		t.Errorf("have %d participants, want 1 (60%% on top of 50%% must be rejected)", len(ps))
	}
}

func TestLoanHandler_AddParticipant_AcceptsUpTo100(t *testing.T) {
	handler, loanRepo, owner, _ := setupLoanHandlerTest(t)
	loanID, _ := loanRepo.Create(&models.Loan{
		UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit, Currency: "DKK", IsActive: true,
	})
	mustAddPart(t, loanRepo, loanID, "Me", 50, true)

	rec := addParticipantRequest(t, handler, owner, loanID, "50")

	if loc := rec.Header().Get("Location"); strings.Contains(loc, "participant=") {
		t.Errorf("Location = %q, want no participant error", loc)
	}
	if ps, _ := loanRepo.GetParticipants(loanID); len(ps) != 2 {
		t.Errorf("have %d participants, want 2", len(ps))
	}
}

func TestLoanHandler_AddParticipant_RejectsInvalidPercentages(t *testing.T) {
	for _, pct := range []string{"0", "-5", "NaN", "Inf", "abc", ""} {
		t.Run(pct, func(t *testing.T) {
			handler, loanRepo, owner, _ := setupLoanHandlerTest(t)
			loanID, _ := loanRepo.Create(&models.Loan{
				UserID: owner.ID, Name: "Apartment", LoanType: models.LoanTypeSplit, Currency: "DKK", IsActive: true,
			})
			mustAddPart(t, loanRepo, loanID, "Me", 50, true)

			rec := addParticipantRequest(t, handler, owner, loanID, pct)

			if loc := rec.Header().Get("Location"); !strings.Contains(loc, "participant=badpct") {
				t.Errorf("Location = %q, want participant=badpct", loc)
			}
			if ps, _ := loanRepo.GetParticipants(loanID); len(ps) != 1 {
				t.Errorf("have %d participants, want 1", len(ps))
			}
		})
	}
}

func TestLoanHandler_Create_RejectsSelfOwnershipOutOfRange(t *testing.T) {
	for _, pct := range []string{"150", "NaN", "-10"} {
		t.Run(pct, func(t *testing.T) {
			handler, loanRepo, owner, _ := setupLoanHandlerTest(t)
			form := url.Values{
				"name": {"Apartment"}, "loan_type": {models.LoanTypeSplit},
				"principal": {"100000"}, "self_ownership_pct": {pct},
			}
			rec := httptest.NewRecorder()
			handler.Create(rec, authedRequest(owner, "/loans", form, nil))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if loans, _ := loanRepo.GetByUserID(owner.ID); len(loans) != 0 {
				t.Errorf("created %d loans, want 0", len(loans))
			}
		})
	}
}

func TestOwnershipWarning(t *testing.T) {
	parts := func(pcts ...float64) []*models.LoanParticipant {
		var ps []*models.LoanParticipant
		for i, pct := range pcts {
			ps = append(ps, &models.LoanParticipant{ID: int64(i + 1), OwnershipPct: pct, IsSelf: i == 0})
		}
		return ps
	}
	split := &models.Loan{LoanType: models.LoanTypeSplit}
	owed := &models.Loan{LoanType: models.LoanTypeOwed}

	if got := ownershipWarning(split, parts(50)); !strings.Contains(got, "50%") {
		t.Errorf("split at 50%%: warning = %q, want it to mention 50%%", got)
	}
	if got := ownershipWarning(split, parts(50, 50)); got != "" {
		t.Errorf("split at 100%%: warning = %q, want none", got)
	}
	if got := ownershipWarning(owed, parts(100)); got != "" {
		t.Errorf("owed loan: warning = %q, want none", got)
	}
}
