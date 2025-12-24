// Package demo provides demo data seeding for demonstration deployments.
package demo

import (
	"log"
	"time"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// Seeder seeds the database with demo data.
type Seeder struct {
	db              *database.DB
	userRepo        *repository.UserRepository
	categoryRepo    *repository.CategoryRepository
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	goalRepo        *repository.GoalRepository
}

// NewSeeder creates a new demo data seeder.
func NewSeeder(db *database.DB) *Seeder {
	return &Seeder{
		db:              db,
		userRepo:        repository.NewUserRepository(db),
		categoryRepo:    repository.NewCategoryRepository(db),
		accountRepo:     repository.NewAccountRepository(db),
		transactionRepo: repository.NewTransactionRepository(db),
		goalRepo:        repository.NewGoalRepository(db),
	}
}

// SeedIfEmpty seeds demo data if the database is empty.
func (s *Seeder) SeedIfEmpty() error {
	count, err := s.userRepo.CountAll()
	if err != nil {
		return err
	}

	if count > 0 {
		log.Println("Database already has users, skipping demo seed")
		return nil
	}

	log.Println("Seeding demo data...")
	return s.Seed()
}

// Seed creates demo user with sample data.
func (s *Seeder) Seed() error {
	// Create demo user
	passwordHash, err := auth.HashPassword("demo1234")
	if err != nil {
		return err
	}

	demoUser := &models.User{
		Email:              "demo@example.com",
		PasswordHash:       passwordHash,
		Name:               "Demo User",
		DefaultCurrency:    "DKK",
		NumberFormat:       "da",
		Theme:              "dark",
		IsAdmin:            true,
		MustChangePassword: false,
	}

	userID, err := s.userRepo.Create(demoUser)
	if err != nil {
		return err
	}
	log.Printf("Created demo user (ID: %d)", userID)

	// Create categories
	categories := []models.Category{
		{UserID: userID, Name: "Aktier", Color: "#6366f1", Icon: "trending-up", SortOrder: 1},
		{UserID: userID, Name: "ETF'er", Color: "#8b5cf6", Icon: "bar-chart-2", SortOrder: 2},
		{UserID: userID, Name: "Pension", Color: "#10b981", Icon: "shield", SortOrder: 3},
		{UserID: userID, Name: "Opsparing", Color: "#f59e0b", Icon: "piggy-bank", SortOrder: 4},
		{UserID: userID, Name: "Krypto", Color: "#ec4899", Icon: "bitcoin", SortOrder: 5},
		{UserID: userID, Name: "Gæld", Color: "#ef4444", Icon: "credit-card", SortOrder: 6},
	}

	categoryIDs := make(map[string]int64)
	for _, cat := range categories {
		id, err := s.categoryRepo.Create(&cat)
		if err != nil {
			return err
		}
		categoryIDs[cat.Name] = id
	}
	log.Printf("Created %d categories", len(categories))

	// Create accounts with realistic Danish investment portfolio
	aktierID := categoryIDs["Aktier"]
	etfID := categoryIDs["ETF'er"]
	pensionID := categoryIDs["Pension"]
	opsparingID := categoryIDs["Opsparing"]
	kryptoID := categoryIDs["Krypto"]
	gaeldID := categoryIDs["Gæld"]

	accounts := []models.Account{
		{UserID: userID, CategoryID: &aktierID, Name: "Nordnet Aktiedepot", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &aktierID, Name: "Saxo Investor", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &etfID, Name: "Nordnet Månedsopsparing", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &pensionID, Name: "Ratepension", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &pensionID, Name: "Aldersopsparing", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &opsparingID, Name: "Nødopsparing", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &opsparingID, Name: "Ferieopsparing", Currency: "DKK", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &kryptoID, Name: "Coinbase", Currency: "USD", IsLiability: false, IsActive: true},
		{UserID: userID, CategoryID: &gaeldID, Name: "Boliglån", Currency: "DKK", IsLiability: true, IsActive: true},
		{UserID: userID, CategoryID: &gaeldID, Name: "Billån", Currency: "DKK", IsLiability: true, IsActive: true},
	}

	accountIDs := make(map[string]int64)
	for _, acc := range accounts {
		id, err := s.accountRepo.Create(&acc)
		if err != nil {
			return err
		}
		accountIDs[acc.Name] = id
	}
	log.Printf("Created %d accounts", len(accounts))

	// Create transactions with realistic growth over 2 years
	now := time.Now()
	transactions := s.generateTransactions(accountIDs, now)

	for _, txn := range transactions {
		_, err := s.transactionRepo.Create(&txn)
		if err != nil {
			return err
		}
	}
	log.Printf("Created %d transactions", len(transactions))

	// Create goals
	deadline2025 := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	deadline2030 := time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC)
	deadline2035 := time.Date(2035, 12, 31, 0, 0, 0, 0, time.UTC)

	goals := []models.Goal{
		{UserID: userID, CategoryID: nil, Name: "Netto formue 1M DKK", TargetAmount: 1000000, TargetCurrency: "DKK", Deadline: &deadline2030},
		{UserID: userID, CategoryID: nil, Name: "Netto formue 2M DKK", TargetAmount: 2000000, TargetCurrency: "DKK", Deadline: &deadline2035},
		{UserID: userID, CategoryID: &aktierID, Name: "Aktiebeholdning 500k", TargetAmount: 500000, TargetCurrency: "DKK", Deadline: &deadline2025},
		{UserID: userID, CategoryID: &pensionID, Name: "Pension 1M DKK", TargetAmount: 1000000, TargetCurrency: "DKK", Deadline: &deadline2035},
		{UserID: userID, CategoryID: &opsparingID, Name: "Nødopsparing 100k", TargetAmount: 100000, TargetCurrency: "DKK", Deadline: &deadline2025},
	}

	for _, goal := range goals {
		_, err := s.goalRepo.Create(&goal)
		if err != nil {
			return err
		}
	}
	log.Printf("Created %d goals", len(goals))

	log.Println("========================================")
	log.Println("DEMO MODE ENABLED")
	log.Println("Login with:")
	log.Println("Email:    demo@example.com")
	log.Println("Password: demo1234")
	log.Println("========================================")

	return nil
}

// generateTransactions creates realistic transaction history.
func (s *Seeder) generateTransactions(accountIDs map[string]int64, now time.Time) []models.Transaction {
	var transactions []models.Transaction

	// Start date: 2 years ago
	startDate := now.AddDate(-2, 0, 0)

	// Nordnet Aktiedepot - growing from 50k to 285k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Nordnet Aktiedepot"],
		startDate, now,
		50000, 285000,
		5000, // Monthly contribution
		"Månedlig indbetaling",
	)...)

	// Saxo Investor - growing from 30k to 142k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Saxo Investor"],
		startDate, now,
		30000, 142000,
		3000,
		"Månedlig investering",
	)...)

	// Nordnet Månedsopsparing (ETF) - growing from 20k to 98k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Nordnet Månedsopsparing"],
		startDate, now,
		20000, 98000,
		2500,
		"Månedsopsparing",
	)...)

	// Ratepension - growing from 150k to 312k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Ratepension"],
		startDate, now,
		150000, 312000,
		4500,
		"Arbejdsgiver indbetaling",
	)...)

	// Aldersopsparing - growing from 25k to 63k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Aldersopsparing"],
		startDate, now,
		25000, 63000,
		1100, // Max yearly is ~5,600 DKK
		"Årlig indbetaling",
	)...)

	// Nødopsparing - relatively stable at 75k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Nødopsparing"],
		startDate, now,
		60000, 75000,
		500,
		"Opsparing",
	)...)

	// Ferieopsparing - cycles between 0 and 25k
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Ferieopsparing"],
		startDate, now,
		5000, 18500,
		1000,
		"Ferieopsparing",
	)...)

	// Coinbase (USD) - volatile crypto
	transactions = append(transactions, s.generateAccountGrowth(
		accountIDs["Coinbase"],
		startDate, now,
		2000, 4250,
		100,
		"BTC purchase",
	)...)

	// Boliglån (liability) - decreasing from 1.8M to 1.72M
	transactions = append(transactions, s.generateLiabilityPaydown(
		accountIDs["Boliglån"],
		startDate, now,
		1800000, 1720000,
		"Afdrag på lån",
	)...)

	// Billån (liability) - decreasing from 180k to 95k
	transactions = append(transactions, s.generateLiabilityPaydown(
		accountIDs["Billån"],
		startDate, now,
		180000, 95000,
		"Billån afdrag",
	)...)

	return transactions
}

// generateAccountGrowth creates transactions showing account growth.
func (s *Seeder) generateAccountGrowth(accountID int64, startDate, endDate time.Time, startBalance, endBalance, monthlyContrib float64, description string) []models.Transaction {
	var transactions []models.Transaction

	months := int(endDate.Sub(startDate).Hours() / 24 / 30)
	if months < 1 {
		months = 1
	}

	// Calculate market return needed beyond contributions
	totalContributions := float64(months) * monthlyContrib
	marketGrowth := endBalance - startBalance - totalContributions

	currentBalance := startBalance
	currentDate := startDate

	// Initial balance
	transactions = append(transactions, models.Transaction{
		AccountID:       accountID,
		Amount:          startBalance,
		BalanceAfter:    currentBalance,
		Description:     "Startbalance",
		TransactionDate: currentDate,
	})

	// Monthly transactions
	for i := 0; i < months; i++ {
		currentDate = currentDate.AddDate(0, 1, 0)
		if currentDate.After(endDate) {
			currentDate = endDate
		}

		// Monthly contribution
		currentBalance += monthlyContrib
		transactions = append(transactions, models.Transaction{
			AccountID:       accountID,
			Amount:          monthlyContrib,
			BalanceAfter:    currentBalance,
			Description:     description,
			TransactionDate: currentDate,
		})

		// Quarterly market movement (simulate market gains/losses)
		if (i+1)%3 == 0 {
			marketMove := marketGrowth / float64(months/3+1)
			// Add some randomness simulation (deterministic based on month)
			adjustment := 1.0 + float64((i%5)-2)*0.1
			marketMove *= adjustment

			currentBalance += marketMove
			if currentBalance < 0 {
				currentBalance = 0
			}
			transactions = append(transactions, models.Transaction{
				AccountID:       accountID,
				Amount:          marketMove,
				BalanceAfter:    currentBalance,
				Description:     "Kursregulering",
				TransactionDate: currentDate.AddDate(0, 0, 15),
			})
		}
	}

	// Adjust final balance to match target
	if len(transactions) > 0 {
		diff := endBalance - currentBalance
		if diff != 0 {
			transactions = append(transactions, models.Transaction{
				AccountID:       accountID,
				Amount:          diff,
				BalanceAfter:    endBalance,
				Description:     "Kursregulering",
				TransactionDate: endDate,
			})
		}
	}

	return transactions
}

// generateLiabilityPaydown creates transactions showing debt paydown.
func (s *Seeder) generateLiabilityPaydown(accountID int64, startDate, endDate time.Time, startBalance, endBalance float64, description string) []models.Transaction {
	var transactions []models.Transaction

	months := int(endDate.Sub(startDate).Hours() / 24 / 30)
	if months < 1 {
		months = 1
	}

	monthlyPayment := (startBalance - endBalance) / float64(months)
	currentBalance := startBalance
	currentDate := startDate

	// Initial balance (stored as positive, displayed as negative for liabilities)
	transactions = append(transactions, models.Transaction{
		AccountID:       accountID,
		Amount:          startBalance,
		BalanceAfter:    currentBalance,
		Description:     "Startbalance",
		TransactionDate: currentDate,
	})

	// Monthly payments
	for i := 0; i < months; i++ {
		currentDate = currentDate.AddDate(0, 1, 0)
		if currentDate.After(endDate) {
			break
		}

		currentBalance -= monthlyPayment
		if currentBalance < endBalance {
			currentBalance = endBalance
		}

		transactions = append(transactions, models.Transaction{
			AccountID:       accountID,
			Amount:          -monthlyPayment,
			BalanceAfter:    currentBalance,
			Description:     description,
			TransactionDate: currentDate,
		})
	}

	return transactions
}
