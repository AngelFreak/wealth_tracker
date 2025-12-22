// Package sync provides broker synchronization functionality.
package sync

import (
	"fmt"
	"log"
	"time"

	"wealth_tracker/internal/broker/nordnet"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// Service orchestrates broker synchronization.
type Service struct {
	connRepo    *repository.BrokerConnectionRepository
	holdingRepo *repository.HoldingRepository
	mappingRepo *repository.AccountMappingRepository
	historyRepo *repository.SyncHistoryRepository
	txnRepo     *repository.TransactionRepository
	scriptDir   string // Directory containing MitID Python scripts
}

// NewService creates a new sync service.
func NewService(
	connRepo *repository.BrokerConnectionRepository,
	holdingRepo *repository.HoldingRepository,
	mappingRepo *repository.AccountMappingRepository,
	historyRepo *repository.SyncHistoryRepository,
	txnRepo *repository.TransactionRepository,
	scriptDir string,
) *Service {
	return &Service{
		connRepo:    connRepo,
		holdingRepo: holdingRepo,
		mappingRepo: mappingRepo,
		historyRepo: historyRepo,
		txnRepo:     txnRepo,
		scriptDir:   scriptDir,
	}
}

// SyncResult contains the result of a sync operation.
type SyncResult struct {
	Success         bool
	AccountsSynced  int
	PositionsSynced int
	Error           error
}

// SyncConnection synchronizes all mapped accounts for a broker connection.
// Routes to the appropriate broker-specific implementation.
func (s *Service) SyncConnection(connectionID int64) (*SyncResult, error) {
	// Get connection to determine broker type
	conn, err := s.connRepo.GetByID(connectionID)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Route to broker-specific sync
	switch conn.BrokerType {
	case "nordnet":
		return s.SyncNordnetConnection(connectionID)
	case "saxo":
		return s.SyncSaxoConnection(connectionID)
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", conn.BrokerType)
	}
}

// SyncNordnetConnection synchronizes all mapped accounts for a Nordnet connection.
// The user must approve the MitID authentication in their app.
func (s *Service) SyncNordnetConnection(connectionID int64) (*SyncResult, error) {
	// Start sync history
	historyID, err := s.historyRepo.Start(connectionID, "full")
	if err != nil {
		return nil, fmt.Errorf("starting sync history: %w", err)
	}

	result := &SyncResult{}

	// Get connection
	conn, err := s.connRepo.GetByID(connectionID)
	if err != nil {
		s.failSync(historyID, connectionID, fmt.Sprintf("getting connection: %v", err))
		return nil, fmt.Errorf("getting connection: %w", err)
	}
	if conn == nil {
		s.failSync(historyID, connectionID, "connection not found")
		return nil, fmt.Errorf("connection not found")
	}

	// Create broker client based on type
	client, err := s.createClient(conn.BrokerType, conn.Country)
	if err != nil {
		s.failSync(historyID, connectionID, fmt.Sprintf("creating client: %v", err))
		return nil, fmt.Errorf("creating client: %w", err)
	}

	// Authenticate using MitID (user must approve in MitID app)
	// Username field stores the MitID user identifier
	// Using native Go implementation instead of Python subprocess
	session, err := nordnet.AuthenticateWithMitIDNative(connectionID, conn.Country, conn.Username, conn.CPR, "APP", s.scriptDir)
	if err != nil {
		s.connRepo.UpdateSyncStatus(connectionID, "auth_failed", err.Error())
		s.failSync(historyID, connectionID, fmt.Sprintf("MitID authentication failed: %v", err))
		return nil, fmt.Errorf("MitID authentication failed: %w", err)
	}

	// Get account mappings
	mappings, err := s.mappingRepo.GetAutoSyncByConnectionID(connectionID)
	if err != nil {
		s.failSync(historyID, connectionID, fmt.Sprintf("getting mappings: %v", err))
		return nil, fmt.Errorf("getting mappings: %w", err)
	}

	// Sync each mapped account
	for _, mapping := range mappings {
		posCount, err := s.syncAccountPositions(client, session, mapping)
		if err != nil {
			// Log error but continue with other accounts
			continue
		}
		result.AccountsSynced++
		result.PositionsSynced += posCount
	}

	// Update connection status
	s.connRepo.UpdateSyncStatus(connectionID, "success", "")

	// Complete sync history
	s.historyRepo.Complete(historyID, result.AccountsSynced, result.PositionsSynced)

	result.Success = true
	return result, nil
}

// syncAccountPositions syncs positions for a single account mapping.
func (s *Service) syncAccountPositions(client *nordnet.Client, session *nordnet.Session, mapping *models.AccountMapping) (int, error) {
	log.Printf("[Sync] Syncing positions for account mapping: LocalAccountID=%d, ExternalAccountID=%s", mapping.LocalAccountID, mapping.ExternalAccountID)

	// Fetch positions from broker
	positions, err := client.GetPositions(session, mapping.ExternalAccountID)
	if err != nil {
		log.Printf("[Sync] Error fetching positions for account %s: %v", mapping.ExternalAccountID, err)
		return 0, fmt.Errorf("fetching positions: %w", err)
	}

	log.Printf("[Sync] Got %d positions for account %s", len(positions), mapping.ExternalAccountID)

	// Fetch ledgers (cash balances) from broker
	ledgers, err := client.GetLedgers(session, mapping.ExternalAccountID)
	if err != nil {
		log.Printf("[Sync] Error fetching ledgers for account %s: %v (continuing without cash)", mapping.ExternalAccountID, err)
		// Don't fail - continue without cash balance
	} else {
		log.Printf("[Sync] Got %d ledgers for account %s", len(ledgers), mapping.ExternalAccountID)
		for _, l := range ledgers {
			log.Printf("[Sync] Ledger: Currency=%s, AccountSum=%.2f %s",
				l.Currency, l.AccountSum.Value, l.AccountSum.Currency)
		}
	}

	syncTime := time.Now()
	var positionsValue float64
	var cashValue float64

	// Upsert each position as a holding
	for _, pos := range positions {
		holding := &models.Holding{
			AccountID:      mapping.LocalAccountID,
			ExternalID:     fmt.Sprintf("%d", pos.InstrumentID()),
			Symbol:         pos.ISIN(),
			Name:           pos.Name(),
			Quantity:       pos.Quantity(),
			AvgPrice:       pos.AcqPriceValue(),
			CurrentPrice:   pos.MarketPriceValue(),
			CurrentValue:   pos.MarketValueAccValue(),
			Currency:       pos.Currency(),
			InstrumentType: pos.InstrumentType(),
			LastUpdated:    syncTime,
		}

		log.Printf("[Sync] Upserting holding: Symbol=%s, Name=%s, Qty=%.2f, Value=%.2f",
			holding.Symbol, holding.Name, holding.Quantity, holding.CurrentValue)

		if err := s.holdingRepo.Upsert(holding); err != nil {
			log.Printf("[Sync] Error upserting holding %s: %v", holding.Symbol, err)
			continue
		}
		positionsValue += pos.MarketValueAccValue()
	}

	// Calculate cash balance from ledgers
	// AccountSum.Value represents cash + pending settlements
	for _, ledger := range ledgers {
		cashValue += ledger.AccountSum.Value
	}

	log.Printf("[Sync] Account %s: Positions=%.2f, Cash=%.2f, Total=%.2f",
		mapping.ExternalAccountID, positionsValue, cashValue, positionsValue+cashValue)

	// Delete stale holdings (positions that no longer exist)
	s.holdingRepo.DeleteStaleHoldings(mapping.LocalAccountID, syncTime)

	// Total value = positions + cash
	totalValue := positionsValue + cashValue

	// Update account balance if it changed
	currentBalance, _ := s.txnRepo.GetLatestBalance(mapping.LocalAccountID)
	if totalValue != currentBalance && (len(positions) > 0 || len(ledgers) > 0) {
		txn := &models.Transaction{
			AccountID:       mapping.LocalAccountID,
			Amount:          totalValue - currentBalance,
			BalanceAfter:    totalValue,
			Description:     "Nordnet sync",
			TransactionDate: syncTime,
		}
		s.txnRepo.Create(txn)
	}

	return len(positions), nil
}

// createClient creates a broker client based on broker type.
func (s *Service) createClient(brokerType, country string) (*nordnet.Client, error) {
	switch brokerType {
	case "nordnet":
		return nordnet.NewClient(country)
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", brokerType)
	}
}

// failSync marks a sync as failed and updates connection status.
func (s *Service) failSync(historyID, connectionID int64, errorMsg string) {
	s.historyRepo.Fail(historyID, errorMsg)
	s.connRepo.UpdateSyncStatus(connectionID, "error", errorMsg)
}

// ExternalAccount is a generic interface for broker accounts.
type ExternalAccount struct {
	ID            string // Unique identifier (AccountKey for Saxo, accid for Nordnet)
	AccountNumber string // Human-readable account number (AccountId for Saxo, accno for Nordnet)
	Name          string
	Currency      string
	Type          string
	Active        bool
}

// GetExternalAccounts fetches accounts from a broker for account mapping setup.
// Routes to the appropriate broker-specific implementation.
func (s *Service) GetExternalAccounts(connectionID int64) ([]ExternalAccount, error) {
	// Get connection
	conn, err := s.connRepo.GetByID(connectionID)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("connection not found")
	}

	switch conn.BrokerType {
	case "nordnet":
		return s.getNordnetExternalAccounts(connectionID, conn)
	case "saxo":
		return s.getSaxoExternalAccountsGeneric(connectionID)
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", conn.BrokerType)
	}
}

// getNordnetExternalAccounts fetches accounts from Nordnet using MitID.
func (s *Service) getNordnetExternalAccounts(connectionID int64, conn *models.BrokerConnection) ([]ExternalAccount, error) {
	// Create client
	client, err := s.createClient(conn.BrokerType, conn.Country)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	// Authenticate using MitID (user must approve in MitID app)
	session, err := nordnet.AuthenticateWithMitIDNative(connectionID, conn.Country, conn.Username, conn.CPR, "APP", s.scriptDir)
	if err != nil {
		return nil, fmt.Errorf("MitID authentication failed: %w", err)
	}

	// Fetch accounts
	accounts, err := client.GetAccounts(session)
	if err != nil {
		return nil, fmt.Errorf("fetching accounts: %w", err)
	}

	// Convert to generic format
	result := make([]ExternalAccount, len(accounts))
	for i, acc := range accounts {
		result[i] = ExternalAccount{
			ID:            acc.AccID.String(),
			AccountNumber: acc.AccNO.String(), // Human-readable account number
			Name:          acc.Alias,
			Currency:      acc.Currency,
			Type:          acc.Type,
			Active:        !acc.IsBlocked,
		}
	}

	return result, nil
}

// getSaxoExternalAccountsGeneric fetches accounts from Saxo using OAuth.
func (s *Service) getSaxoExternalAccountsGeneric(connectionID int64) ([]ExternalAccount, error) {
	accounts, err := s.GetSaxoExternalAccounts(connectionID)
	if err != nil {
		return nil, err
	}

	// Convert to generic format
	result := make([]ExternalAccount, len(accounts))
	for i, acc := range accounts {
		// Use DisplayName if available, otherwise fall back to AccountID
		name := acc.DisplayName
		if name == "" {
			name = acc.AccountID
		}
		result[i] = ExternalAccount{
			ID:            acc.AccountKey,
			AccountNumber: acc.AccountID, // Human-readable like "140459ASK"
			Name:          name,
			Currency:      acc.Currency,
			Type:          acc.AccountType,
			Active:        acc.Active,
		}
	}

	return result, nil
}

// TestConnection is no longer supported for interactive auth brokers.
// Returns nil since testing requires user interaction (MitID or OAuth).
func (s *Service) TestConnection(brokerType, country, username, password string) error {
	// Interactive authentication doesn't support pre-testing
	// The user will authenticate when they first sync or map accounts
	switch brokerType {
	case "nordnet":
		// Nordnet uses MitID - can't test without user interaction
		return nil
	case "saxo":
		// Saxo uses OAuth - can't test without user interaction
		return nil
	default:
		return fmt.Errorf("unsupported broker type: %s", brokerType)
	}
}
