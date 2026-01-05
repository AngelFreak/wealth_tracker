// Package handlers provides HTTP handlers for the wealth tracker.
package handlers

import (
	"html/template"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/repository"
	"wealth_tracker/internal/services"
	"wealth_tracker/internal/sync"
)

// Dependencies holds all handler dependencies.
// This reduces constructor parameter lists and simplifies dependency injection.
type Dependencies struct {
	// Templates
	Templates map[string]*template.Template

	// Repositories
	UserRepo             *repository.UserRepository
	AccountRepo          *repository.AccountRepository
	CategoryRepo         *repository.CategoryRepository
	TransactionRepo      *repository.TransactionRepository
	GoalRepo             *repository.GoalRepository
	BrokerConnectionRepo *repository.BrokerConnectionRepository
	AccountMappingRepo   *repository.AccountMappingRepository
	HoldingRepo          *repository.HoldingRepository
	SyncHistoryRepo      *repository.SyncHistoryRepository
	AllocationTargetRepo *repository.AllocationTargetRepository

	// Services
	SessionManager   *auth.SessionManager
	SyncService      *sync.Service
	PortfolioService *services.PortfolioService
	AuditService     *services.AuditService
	CurrencyService  *services.CurrencyService
}

// NewDependencies creates an empty Dependencies container.
// Use the builder pattern to set required dependencies.
func NewDependencies() *Dependencies {
	return &Dependencies{}
}

// WithTemplates sets the template map.
func (d *Dependencies) WithTemplates(t map[string]*template.Template) *Dependencies {
	d.Templates = t
	return d
}

// WithUserRepo sets the user repository.
func (d *Dependencies) WithUserRepo(r *repository.UserRepository) *Dependencies {
	d.UserRepo = r
	return d
}

// WithAccountRepo sets the account repository.
func (d *Dependencies) WithAccountRepo(r *repository.AccountRepository) *Dependencies {
	d.AccountRepo = r
	return d
}

// WithCategoryRepo sets the category repository.
func (d *Dependencies) WithCategoryRepo(r *repository.CategoryRepository) *Dependencies {
	d.CategoryRepo = r
	return d
}

// WithTransactionRepo sets the transaction repository.
func (d *Dependencies) WithTransactionRepo(r *repository.TransactionRepository) *Dependencies {
	d.TransactionRepo = r
	return d
}

// WithGoalRepo sets the goal repository.
func (d *Dependencies) WithGoalRepo(r *repository.GoalRepository) *Dependencies {
	d.GoalRepo = r
	return d
}

// WithBrokerConnectionRepo sets the broker connection repository.
func (d *Dependencies) WithBrokerConnectionRepo(r *repository.BrokerConnectionRepository) *Dependencies {
	d.BrokerConnectionRepo = r
	return d
}

// WithAccountMappingRepo sets the account mapping repository.
func (d *Dependencies) WithAccountMappingRepo(r *repository.AccountMappingRepository) *Dependencies {
	d.AccountMappingRepo = r
	return d
}

// WithHoldingRepo sets the holding repository.
func (d *Dependencies) WithHoldingRepo(r *repository.HoldingRepository) *Dependencies {
	d.HoldingRepo = r
	return d
}

// WithSyncHistoryRepo sets the sync history repository.
func (d *Dependencies) WithSyncHistoryRepo(r *repository.SyncHistoryRepository) *Dependencies {
	d.SyncHistoryRepo = r
	return d
}

// WithAllocationTargetRepo sets the allocation target repository.
func (d *Dependencies) WithAllocationTargetRepo(r *repository.AllocationTargetRepository) *Dependencies {
	d.AllocationTargetRepo = r
	return d
}

// WithSessionManager sets the session manager.
func (d *Dependencies) WithSessionManager(sm *auth.SessionManager) *Dependencies {
	d.SessionManager = sm
	return d
}

// WithSyncService sets the sync service.
func (d *Dependencies) WithSyncService(s *sync.Service) *Dependencies {
	d.SyncService = s
	return d
}

// WithPortfolioService sets the portfolio service.
func (d *Dependencies) WithPortfolioService(s *services.PortfolioService) *Dependencies {
	d.PortfolioService = s
	return d
}

// WithAuditService sets the audit service.
func (d *Dependencies) WithAuditService(s *services.AuditService) *Dependencies {
	d.AuditService = s
	return d
}

// WithCurrencyService sets the currency service.
func (d *Dependencies) WithCurrencyService(s *services.CurrencyService) *Dependencies {
	d.CurrencyService = s
	return d
}
