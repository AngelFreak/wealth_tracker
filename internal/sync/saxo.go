package sync

import (
	"fmt"
	"log"
	"time"

	"wealth_tracker/internal/broker/saxo"
	"wealth_tracker/internal/models"
)

// SyncSaxoConnection synchronizes all mapped accounts for a Saxo broker connection.
// Uses OAuth2 PKCE authentication (browser-based login).
func (s *Service) SyncSaxoConnection(connectionID int64) (*SyncResult, error) {
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

	// Get or refresh OAuth session
	session, err := saxo.GetOrRefreshSession(connectionID)
	if err != nil {
		// Need new OAuth authentication
		log.Printf("[Saxo Sync] No valid session, starting OAuth flow for connection %d", connectionID)
		session, err = saxo.AuthenticateWithOAuth(connectionID, conn.AppKey, conn.AppSecret, conn.RedirectURI)
		if err != nil {
			s.connRepo.UpdateSyncStatus(connectionID, "auth_failed", err.Error())
			s.failSync(historyID, connectionID, fmt.Sprintf("OAuth authentication failed: %v", err))
			return nil, fmt.Errorf("OAuth authentication failed: %w", err)
		}
	}

	// Create Saxo client
	client := saxo.NewClient()

	// Get account mappings
	mappings, err := s.mappingRepo.GetAutoSyncByConnectionID(connectionID)
	if err != nil {
		s.failSync(historyID, connectionID, fmt.Sprintf("getting mappings: %v", err))
		return nil, fmt.Errorf("getting mappings: %w", err)
	}

	// Sync each mapped account
	for _, mapping := range mappings {
		posCount, err := s.syncSaxoAccountPositions(client, session, mapping)
		if err != nil {
			log.Printf("[Saxo Sync] Error syncing account %s: %v", mapping.ExternalAccountID, err)
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

// syncSaxoAccountPositions syncs positions for a single Saxo account mapping.
func (s *Service) syncSaxoAccountPositions(client *saxo.Client, session *saxo.Session, mapping *models.AccountMapping) (int, error) {
	log.Printf("[Saxo Sync] Syncing positions for account mapping: LocalAccountID=%d, ExternalAccountID=%s", mapping.LocalAccountID, mapping.ExternalAccountID)

	// ExternalAccountID for Saxo is the AccountKey
	accountKey := mapping.ExternalAccountID

	// Fetch positions with instrument details
	positions, err := client.GetPositionsWithDetails(session, accountKey)
	if err != nil {
		log.Printf("[Saxo Sync] Error fetching positions for account %s: %v", accountKey, err)
		return 0, fmt.Errorf("fetching positions: %w", err)
	}

	log.Printf("[Saxo Sync] Got %d positions for account %s", len(positions), accountKey)

	// Fetch balance
	balance, err := client.GetBalance(session, accountKey)
	if err != nil {
		log.Printf("[Saxo Sync] Error fetching balance for account %s: %v (continuing without cash)", accountKey, err)
		// Don't fail - continue without cash balance
	} else {
		log.Printf("[Saxo Sync] Balance for account %s: Cash=%.2f %s", accountKey, balance.CashBalance, balance.Currency)
	}

	syncTime := time.Now()
	var positionsValue float64
	var cashValue float64

	// Get positions value from balance API (more reliable than summing positions when markets closed)
	var positionsValueFromBalance float64
	if balance != nil {
		// NonMarginPositionsValue is the total value of non-margin positions (stocks, ETFs, etc.)
		// Use it if available, otherwise calculate from TotalValue - CashBalance
		if balance.NonMarginPositionsValue > 0 {
			positionsValueFromBalance = balance.NonMarginPositionsValue
		} else if balance.TotalValue > 0 && balance.CashBalance >= 0 {
			positionsValueFromBalance = balance.TotalValue - balance.CashBalance
		}
		log.Printf("[Saxo Sync] Balance API positions value: NonMargin=%.2f, Calculated=%.2f",
			balance.NonMarginPositionsValue, balance.TotalValue-balance.CashBalance)
	}

	// Calculate total cost basis for proportional value distribution
	var totalCostBasis float64
	for _, pos := range positions {
		totalCostBasis += pos.AbsQuantity() * pos.OpenPrice()
	}

	// Upsert each position as a holding
	for _, pos := range positions {
		// Debug: log all price/value fields to see what's available
		log.Printf("[Saxo Sync] Position %s raw values: CurrentPrice=%.4f, MarketValue=%.2f, MarketValueInBase=%.2f, Exposure=%.2f, ExposureInBase=%.2f, OpenPrice=%.4f",
			pos.Symbol(),
			pos.Position.PositionView.CurrentPrice,
			pos.Position.PositionView.MarketValue,
			pos.Position.PositionView.MarketValueInBaseCurrency,
			pos.Position.PositionView.Exposure,
			pos.Position.PositionView.ExposureInBaseCurrency,
			pos.Position.PositionBase.OpenPrice)

		// Calculate holding value - use API value if available, otherwise estimate from balance
		holdingValue := pos.MarketValue()
		holdingPrice := pos.CurrentPrice()

		// If API returns 0 values (markets closed), estimate from balance proportionally
		if holdingValue == 0 && positionsValueFromBalance > 0 && totalCostBasis > 0 {
			costBasis := pos.AbsQuantity() * pos.OpenPrice()
			// Distribute total positions value proportionally based on cost basis
			holdingValue = (costBasis / totalCostBasis) * positionsValueFromBalance
			// Calculate implied price from value
			if pos.AbsQuantity() > 0 {
				holdingPrice = holdingValue / pos.AbsQuantity()
			}
			log.Printf("[Saxo Sync] Estimated value for %s: CostBasis=%.2f, Proportion=%.4f, Value=%.2f",
				pos.Symbol(), costBasis, costBasis/totalCostBasis, holdingValue)
		}

		holding := &models.Holding{
			AccountID:      mapping.LocalAccountID,
			ExternalID:     fmt.Sprintf("%d", pos.Uic()),
			Symbol:         pos.Symbol(),
			Name:           pos.Name(),
			Quantity:       pos.AbsQuantity(), // Use absolute for display
			AvgPrice:       pos.OpenPrice(),
			CurrentPrice:   holdingPrice,
			CurrentValue:   holdingValue,
			Currency:       pos.Currency(),
			InstrumentType: pos.AssetType(),
			LastUpdated:    syncTime,
		}

		log.Printf("[Saxo Sync] Upserting holding: Symbol=%s, Name=%s, Qty=%.2f, Price=%.4f, Value=%.2f",
			holding.Symbol, holding.Name, holding.Quantity, holding.CurrentPrice, holding.CurrentValue)

		if err := s.holdingRepo.Upsert(holding); err != nil {
			log.Printf("[Saxo Sync] Error upserting holding %s: %v", holding.Symbol, err)
			continue
		}
		positionsValue += holdingValue
	}

	// Get total value from balance response
	// Use TotalValue from balance API as it's more accurate than summing positions
	// (positions may return 0 for market values when markets are closed)
	var totalValue float64
	if balance != nil {
		totalValue = balance.TotalValue
		cashValue = balance.CashBalance
	} else {
		// Fallback to positions + cash if balance not available
		totalValue = positionsValue + cashValue
	}

	log.Printf("[Saxo Sync] Account %s: Positions=%.2f, Cash=%.2f, BalanceTotalValue=%.2f",
		accountKey, positionsValue, cashValue, totalValue)

	// Delete stale holdings (positions that no longer exist)
	s.holdingRepo.DeleteStaleHoldings(mapping.LocalAccountID, syncTime)

	// Update account balance if it changed
	currentBalance, _ := s.txnRepo.GetLatestBalance(mapping.LocalAccountID)
	if totalValue != currentBalance && (len(positions) > 0 || balance != nil) {
		txn := &models.Transaction{
			AccountID:       mapping.LocalAccountID,
			Amount:          totalValue - currentBalance,
			BalanceAfter:    totalValue,
			Description:     "Saxo sync",
			TransactionDate: syncTime,
		}
		s.txnRepo.Create(txn)
	}

	return len(positions), nil
}

// GetSaxoExternalAccounts fetches accounts from Saxo for account mapping setup.
// Uses OAuth2 authentication (browser-based login).
func (s *Service) GetSaxoExternalAccounts(connectionID int64) ([]saxo.Account, error) {
	// Get connection for AppKey
	conn, err := s.connRepo.GetByID(connectionID)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Get or refresh OAuth session
	session, err := saxo.GetOrRefreshSession(connectionID)
	if err != nil {
		// Need new OAuth authentication
		log.Printf("[Saxo Sync] No valid session, starting OAuth flow for connection %d", connectionID)
		session, err = saxo.AuthenticateWithOAuth(connectionID, conn.AppKey, conn.AppSecret, conn.RedirectURI)
		if err != nil {
			return nil, fmt.Errorf("OAuth authentication failed: %w", err)
		}
	}

	// Create Saxo client
	client := saxo.NewClient()

	// Fetch accounts
	accounts, err := client.GetAccounts(session)
	if err != nil {
		return nil, fmt.Errorf("fetching accounts: %w", err)
	}

	return accounts, nil
}

// GetSaxoOAuthStatus returns the OAuth authentication status for a Saxo connection.
func (s *Service) GetSaxoOAuthStatus(connectionID int64) string {
	return saxo.GetOAuthStatus(connectionID)
}

// GetSaxoOAuthURL returns the OAuth authorization URL if an authentication is in progress.
func (s *Service) GetSaxoOAuthURL(connectionID int64) string {
	return saxo.GetOAuthURL(connectionID)
}

// StartSaxoOAuth initiates an OAuth flow for a Saxo connection.
// Returns immediately after starting the flow - use GetSaxoOAuthStatus to poll for completion.
func (s *Service) StartSaxoOAuth(connectionID int64) error {
	// Get connection for AppKey
	conn, err := s.connRepo.GetByID(connectionID)
	if err != nil {
		return fmt.Errorf("getting connection: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("connection not found")
	}

	// Clear any stale OAuth session before starting new one
	saxo.ClearActiveOAuthSession(connectionID)

	appKey := conn.AppKey
	appSecret := conn.AppSecret
	redirectURI := conn.RedirectURI
	go func() {
		session, err := saxo.AuthenticateWithOAuth(connectionID, appKey, appSecret, redirectURI)
		if err != nil {
			log.Printf("[Saxo Sync] OAuth authentication failed for connection %d: %v", connectionID, err)
			s.connRepo.UpdateSyncStatus(connectionID, "auth_failed", err.Error())
			return
		}
		log.Printf("[Saxo Sync] OAuth authentication successful for connection %d", connectionID)
		// Session is cached automatically by AuthenticateWithOAuth
		_ = session
	}()
	return nil
}
