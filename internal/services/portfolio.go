// Package services contains business logic for the wealth tracker.
package services

import (
	"math"
	"sort"
	"strconv"

	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// PortfolioService handles portfolio composition and rebalancing calculations.
type PortfolioService struct {
	accountRepo     *repository.AccountRepository
	holdingRepo     *repository.HoldingRepository
	categoryRepo    *repository.CategoryRepository
	transactionRepo *repository.TransactionRepository
	targetRepo      *repository.AllocationTargetRepository
	currencyService *CurrencyService
	baseCurrency    string
}

// NewPortfolioService creates a new PortfolioService.
func NewPortfolioService(
	accountRepo *repository.AccountRepository,
	holdingRepo *repository.HoldingRepository,
	categoryRepo *repository.CategoryRepository,
	transactionRepo *repository.TransactionRepository,
	targetRepo *repository.AllocationTargetRepository,
) *PortfolioService {
	return &PortfolioService{
		accountRepo:     accountRepo,
		holdingRepo:     holdingRepo,
		categoryRepo:    categoryRepo,
		transactionRepo: transactionRepo,
		targetRepo:      targetRepo,
		baseCurrency:    "DKK", // Default base currency
	}
}

// NewPortfolioServiceWithCurrency creates a PortfolioService with currency conversion support.
func NewPortfolioServiceWithCurrency(
	accountRepo *repository.AccountRepository,
	holdingRepo *repository.HoldingRepository,
	categoryRepo *repository.CategoryRepository,
	transactionRepo *repository.TransactionRepository,
	targetRepo *repository.AllocationTargetRepository,
	currencyService *CurrencyService,
	baseCurrency string,
) *PortfolioService {
	if baseCurrency == "" {
		baseCurrency = "DKK"
	}
	return &PortfolioService{
		accountRepo:     accountRepo,
		holdingRepo:     holdingRepo,
		categoryRepo:    categoryRepo,
		transactionRepo: transactionRepo,
		targetRepo:      targetRepo,
		currencyService: currencyService,
		baseCurrency:    baseCurrency,
	}
}

// convertToBase converts an amount from the given currency to the base currency.
func (s *PortfolioService) convertToBase(amount float64, currency string) float64 {
	if s.currencyService == nil || currency == s.baseCurrency || currency == "" {
		return amount
	}
	converted, err := s.currencyService.Convert(amount, currency, s.baseCurrency)
	if err != nil {
		// If conversion fails, return original amount
		return amount
	}
	return converted
}

// PortfolioComposition represents the breakdown of a portfolio.
type PortfolioComposition struct {
	TotalValue       float64                     `json:"total_value"`
	TotalPositions   int                         `json:"total_positions"`
	BaseCurrency     string                      `json:"base_currency"`
	ByCategory       []CategoryAllocation        `json:"by_category"`
	ByAssetType      []AssetTypeAllocation       `json:"by_asset_type"`
	ByCurrency       []CurrencyAllocation        `json:"by_currency"`
	Holdings         []HoldingAllocation         `json:"holdings"`
	TopHolding       *HoldingAllocation          `json:"top_holding,omitempty"`
	ConcentrationPct float64                     `json:"concentration_pct"` // Top 5 holdings %
}

// CategoryAllocation represents allocation to a category.
type CategoryAllocation struct {
	CategoryID   int64   `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Color        string  `json:"color"`
	Value        float64 `json:"value"`
	Percentage   float64 `json:"percentage"`
	AccountCount int     `json:"account_count"`
}

// AssetTypeAllocation represents allocation to an asset type.
type AssetTypeAllocation struct {
	AssetType  string  `json:"asset_type"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
	Count      int     `json:"count"`
}

// CurrencyAllocation represents allocation by currency.
type CurrencyAllocation struct {
	Currency   string  `json:"currency"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
}

// HoldingAllocation represents a single holding's allocation.
type HoldingAllocation struct {
	AccountID      int64   `json:"account_id"`
	AccountName    string  `json:"account_name"`
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name"`
	Value          float64 `json:"value"`           // Value in original currency
	ValueInBase    float64 `json:"value_in_base"`   // Value converted to base currency
	Percentage     float64 `json:"percentage"`
	ProfitLoss     float64 `json:"profit_loss"`
	ProfitLossPct  float64 `json:"profit_loss_pct"`
	InstrumentType string  `json:"instrument_type"`
	Currency       string  `json:"currency"`
}

// AllocationComparison compares actual vs target allocation.
type AllocationComparison struct {
	TargetType string                   `json:"target_type"`
	Items      []AllocationComparisonItem `json:"items"`
	TotalPct   float64                  `json:"total_pct"` // Sum of target percentages
}

// AllocationComparisonItem represents one item in the comparison.
type AllocationComparisonItem struct {
	Key        string  `json:"key"`         // Category ID, asset type, or currency
	Name       string  `json:"name"`        // Display name
	Color      string  `json:"color"`       // For categories
	ActualPct  float64 `json:"actual_pct"`
	TargetPct  float64 `json:"target_pct"`
	DriftPct   float64 `json:"drift_pct"`   // Actual - Target
	ActualValue float64 `json:"actual_value"`
}

// RebalanceRecommendation provides rebalancing suggestions.
type RebalanceRecommendation struct {
	NewMoney      float64                  `json:"new_money"`
	Recommendations []RebalanceAction      `json:"recommendations"`
	TaxTips       []TaxTip                 `json:"tax_tips"`
}

// RebalanceAction represents a buy/sell action.
type RebalanceAction struct {
	TargetKey   string  `json:"target_key"`
	TargetName  string  `json:"target_name"`
	Action      string  `json:"action"` // "buy", "sell", "hold"
	Amount      float64 `json:"amount"`
	CurrentPct  float64 `json:"current_pct"`
	TargetPct   float64 `json:"target_pct"`
	TaxImpact   string  `json:"tax_impact,omitempty"`
}

// TaxTip provides Danish tax optimization advice.
type TaxTip struct {
	Priority    int    `json:"priority"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action,omitempty"`
}

// Danish tax constants (2025)
const (
	ASKMaxDeposit         = 166200.0 // Max ASK contribution 2025
	StockGainThreshold    = 67500.0  // Threshold for 27% vs 42% tax
	StockGainLowRate      = 27.0     // Tax rate below threshold
	StockGainHighRate     = 42.0     // Tax rate above threshold
	ASKTaxRate            = 17.0     // Flat tax on ASK
	PensionWithdrawalRate = 37.0     // Approximate pension income tax
)

// inferAssetTypeFromCategory uses the category name as the asset type.
// This is used when accounts have balances but no broker holdings synced.
func inferAssetTypeFromCategory(categoryName string) string {
	if categoryName == "" || categoryName == "Uncategorized" {
		return "other"
	}
	return categoryName
}

// inferSymbolFromCategory returns a placeholder symbol for accounts without holdings.
// Uses a generic marker that the frontend can detect to show account name instead.
func inferSymbolFromCategory(categoryName string) string {
	return "ACCOUNT" // Generic marker for category-based positions
}

// GetPortfolioComposition calculates the current portfolio breakdown.
func (s *PortfolioService) GetPortfolioComposition(userID int64) (*PortfolioComposition, error) {
	// Get all active accounts (assets only, not liabilities)
	accounts, err := s.accountRepo.GetByUserIDActiveOnly(userID)
	if err != nil {
		return nil, err
	}

	// Get categories for lookup
	categories, err := s.categoryRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	categoryMap := make(map[int64]*models.Category)
	for _, c := range categories {
		categoryMap[c.ID] = c
	}

	// Build composition
	composition := &PortfolioComposition{
		BaseCurrency: s.baseCurrency,
		ByCategory:   make([]CategoryAllocation, 0),
		ByAssetType:  make([]AssetTypeAllocation, 0),
		ByCurrency:   make([]CurrencyAllocation, 0),
		Holdings:     make([]HoldingAllocation, 0),
	}

	categoryTotals := make(map[int64]*CategoryAllocation)
	assetTypeTotals := make(map[string]*AssetTypeAllocation)
	currencyTotals := make(map[string]*CurrencyAllocation)

	for _, account := range accounts {
		// Skip liabilities
		if account.IsLiability {
			continue
		}

		// Get account balance from latest transaction
		balance, err := s.transactionRepo.GetLatestBalance(account.ID)
		if err != nil {
			balance = 0
		}

		// Get holdings for this account
		holdings, err := s.holdingRepo.GetByAccountID(account.ID)
		if err != nil {
			holdings = []*models.Holding{}
		}

		// Calculate account value (holdings or balance)
		accountValue := balance
		if len(holdings) > 0 {
			holdingsTotal := 0.0
			for _, h := range holdings {
				holdingsTotal += h.CurrentValue
			}
			if holdingsTotal > 0 {
				accountValue = holdingsTotal
			}
		}

		composition.TotalValue += accountValue

		// Category allocation
		catID := int64(0)
		if account.CategoryID != nil {
			catID = *account.CategoryID
		}
		if _, exists := categoryTotals[catID]; !exists {
			catName := "Uncategorized"
			catColor := "#6b7280"
			if cat, ok := categoryMap[catID]; ok && catID > 0 {
				catName = cat.Name
				catColor = cat.Color
			}
			categoryTotals[catID] = &CategoryAllocation{
				CategoryID:   catID,
				CategoryName: catName,
				Color:        catColor,
			}
		}
		categoryTotals[catID].Value += accountValue
		categoryTotals[catID].AccountCount++

		// Process holdings for asset type and individual allocations
		for _, h := range holdings {
			composition.TotalPositions++

			// Determine currency
			currency := h.Currency
			if currency == "" {
				currency = account.Currency
			}

			// Convert to base currency for aggregation
			valueInBase := s.convertToBase(h.CurrentValue, currency)

			// Asset type
			assetType := h.InstrumentType
			if assetType == "" {
				assetType = "unknown"
			}
			if _, exists := assetTypeTotals[assetType]; !exists {
				assetTypeTotals[assetType] = &AssetTypeAllocation{
					AssetType: assetType,
				}
			}
			assetTypeTotals[assetType].Value += valueInBase
			assetTypeTotals[assetType].Count++

			// Currency allocation (in original currency for exposure tracking)
			if _, exists := currencyTotals[currency]; !exists {
				currencyTotals[currency] = &CurrencyAllocation{
					Currency: currency,
				}
			}
			currencyTotals[currency].Value += valueInBase

			// Individual holding
			composition.Holdings = append(composition.Holdings, HoldingAllocation{
				AccountID:      account.ID,
				AccountName:    account.Name,
				Symbol:         h.Symbol,
				Name:           h.Name,
				Value:          h.CurrentValue,
				ValueInBase:    valueInBase,
				ProfitLoss:     h.ProfitLoss(),
				ProfitLossPct:  h.ProfitLossPercent(),
				InstrumentType: h.InstrumentType,
				Currency:       currency,
			})
		}

		// If no holdings, add account balance - infer asset type from category
		if len(holdings) == 0 && balance > 0 {
			composition.TotalPositions++

			currency := account.Currency
			valueInBase := s.convertToBase(balance, currency)

			// Infer asset type from category name
			assetType := inferAssetTypeFromCategory(categoryTotals[catID].CategoryName)
			if _, exists := assetTypeTotals[assetType]; !exists {
				assetTypeTotals[assetType] = &AssetTypeAllocation{
					AssetType: assetType,
				}
			}
			assetTypeTotals[assetType].Value += valueInBase
			assetTypeTotals[assetType].Count++

			if _, exists := currencyTotals[currency]; !exists {
				currencyTotals[currency] = &CurrencyAllocation{
					Currency: currency,
				}
			}
			currencyTotals[currency].Value += valueInBase

			// Use category-based symbol instead of generic CASH
			symbol := inferSymbolFromCategory(categoryTotals[catID].CategoryName)
			composition.Holdings = append(composition.Holdings, HoldingAllocation{
				AccountID:      account.ID,
				AccountName:    account.Name,
				Symbol:         symbol,
				Name:           account.Name,
				Value:          balance,
				ValueInBase:    valueInBase,
				InstrumentType: assetType,
				Currency:       currency,
			})
		}
	}

	// Calculate percentages
	for _, cat := range categoryTotals {
		if composition.TotalValue > 0 {
			cat.Percentage = (cat.Value / composition.TotalValue) * 100
		}
		composition.ByCategory = append(composition.ByCategory, *cat)
	}
	sort.Slice(composition.ByCategory, func(i, j int) bool {
		return composition.ByCategory[i].Value > composition.ByCategory[j].Value
	})

	for _, at := range assetTypeTotals {
		if composition.TotalValue > 0 {
			at.Percentage = (at.Value / composition.TotalValue) * 100
		}
		composition.ByAssetType = append(composition.ByAssetType, *at)
	}
	sort.Slice(composition.ByAssetType, func(i, j int) bool {
		return composition.ByAssetType[i].Value > composition.ByAssetType[j].Value
	})

	for _, cur := range currencyTotals {
		if composition.TotalValue > 0 {
			cur.Percentage = (cur.Value / composition.TotalValue) * 100
		}
		composition.ByCurrency = append(composition.ByCurrency, *cur)
	}
	sort.Slice(composition.ByCurrency, func(i, j int) bool {
		return composition.ByCurrency[i].Value > composition.ByCurrency[j].Value
	})

	// Sort holdings by value and calculate percentages
	sort.Slice(composition.Holdings, func(i, j int) bool {
		return composition.Holdings[i].Value > composition.Holdings[j].Value
	})
	for i := range composition.Holdings {
		if composition.TotalValue > 0 {
			composition.Holdings[i].Percentage = (composition.Holdings[i].Value / composition.TotalValue) * 100
		}
	}

	// Top holding and concentration
	if len(composition.Holdings) > 0 {
		top := composition.Holdings[0]
		composition.TopHolding = &top

		// Top 5 concentration
		top5Value := 0.0
		for i := 0; i < min(5, len(composition.Holdings)); i++ {
			top5Value += composition.Holdings[i].Value
		}
		if composition.TotalValue > 0 {
			composition.ConcentrationPct = (top5Value / composition.TotalValue) * 100
		}
	}

	return composition, nil
}

// GetAllocationComparison compares actual allocation to targets.
func (s *PortfolioService) GetAllocationComparison(userID int64, targetType string) (*AllocationComparison, error) {
	composition, err := s.GetPortfolioComposition(userID)
	if err != nil {
		return nil, err
	}

	targets, err := s.targetRepo.GetByUserIDAndType(userID, targetType)
	if err != nil {
		return nil, err
	}

	comparison := &AllocationComparison{
		TargetType: targetType,
		Items:      make([]AllocationComparisonItem, 0),
	}

	// Build target map
	targetMap := make(map[string]*models.AllocationTarget)
	for _, t := range targets {
		targetMap[t.TargetKey] = t
		comparison.TotalPct += t.TargetPct
	}

	// Track which targets have been matched
	matchedTargets := make(map[string]bool)

	// Compare based on type
	switch targetType {
	case models.TargetTypeCategory:
		for _, cat := range composition.ByCategory {
			key := strconv.FormatInt(cat.CategoryID, 10)
			item := AllocationComparisonItem{
				Key:         key,
				Name:        cat.CategoryName,
				Color:       cat.Color,
				ActualPct:   cat.Percentage,
				ActualValue: cat.Value,
			}
			if target, ok := targetMap[key]; ok {
				item.TargetPct = target.TargetPct
				item.DriftPct = item.ActualPct - item.TargetPct
				matchedTargets[key] = true
			}
			comparison.Items = append(comparison.Items, item)
		}
		// Add targets without holdings
		for _, t := range targets {
			if !matchedTargets[t.TargetKey] {
				comparison.Items = append(comparison.Items, AllocationComparisonItem{
					Key:       t.TargetKey,
					Name:      "Category " + t.TargetKey, // Fallback name
					TargetPct: t.TargetPct,
					DriftPct:  -t.TargetPct, // 0% actual - target%
				})
			}
		}

	case models.TargetTypeAssetType:
		for _, at := range composition.ByAssetType {
			item := AllocationComparisonItem{
				Key:         at.AssetType,
				Name:        at.AssetType,
				ActualPct:   at.Percentage,
				ActualValue: at.Value,
			}
			if target, ok := targetMap[at.AssetType]; ok {
				item.TargetPct = target.TargetPct
				item.DriftPct = item.ActualPct - item.TargetPct
				matchedTargets[at.AssetType] = true
			}
			comparison.Items = append(comparison.Items, item)
		}
		// Add targets without holdings
		for _, t := range targets {
			if !matchedTargets[t.TargetKey] {
				comparison.Items = append(comparison.Items, AllocationComparisonItem{
					Key:       t.TargetKey,
					Name:      t.TargetKey,
					TargetPct: t.TargetPct,
					DriftPct:  -t.TargetPct, // 0% actual - target%
				})
			}
		}

	case models.TargetTypeCurrency:
		for _, cur := range composition.ByCurrency {
			item := AllocationComparisonItem{
				Key:         cur.Currency,
				Name:        cur.Currency,
				ActualPct:   cur.Percentage,
				ActualValue: cur.Value,
			}
			if target, ok := targetMap[cur.Currency]; ok {
				item.TargetPct = target.TargetPct
				item.DriftPct = item.ActualPct - item.TargetPct
				matchedTargets[cur.Currency] = true
			}
			comparison.Items = append(comparison.Items, item)
		}
		// Add targets without holdings
		for _, t := range targets {
			if !matchedTargets[t.TargetKey] {
				comparison.Items = append(comparison.Items, AllocationComparisonItem{
					Key:       t.TargetKey,
					Name:      t.TargetKey,
					TargetPct: t.TargetPct,
					DriftPct:  -t.TargetPct, // 0% actual - target%
				})
			}
		}
	}

	return comparison, nil
}

// CalculateRebalancing calculates buy/sell recommendations to reach target allocation.
func (s *PortfolioService) CalculateRebalancing(userID int64, targetType string, newMoney float64) (*RebalanceRecommendation, error) {
	composition, err := s.GetPortfolioComposition(userID)
	if err != nil {
		return nil, err
	}

	targets, err := s.targetRepo.GetByUserIDAndType(userID, targetType)
	if err != nil {
		return nil, err
	}

	recommendation := &RebalanceRecommendation{
		NewMoney:        newMoney,
		Recommendations: make([]RebalanceAction, 0),
		TaxTips:         make([]TaxTip, 0),
	}

	// Build target map
	targetMap := make(map[string]float64)
	for _, t := range targets {
		targetMap[t.TargetKey] = t.TargetPct
	}

	// New total after adding money
	newTotal := composition.TotalValue + newMoney

	// Get current allocations based on type
	var allocations map[string]float64
	var names map[string]string

	switch targetType {
	case models.TargetTypeCategory:
		allocations = make(map[string]float64)
		names = make(map[string]string)
		for _, cat := range composition.ByCategory {
			key := strconv.FormatInt(cat.CategoryID, 10)
			allocations[key] = cat.Value
			names[key] = cat.CategoryName
		}

	case models.TargetTypeAssetType:
		allocations = make(map[string]float64)
		names = make(map[string]string)
		for _, at := range composition.ByAssetType {
			allocations[at.AssetType] = at.Value
			names[at.AssetType] = at.AssetType
		}

	case models.TargetTypeCurrency:
		allocations = make(map[string]float64)
		names = make(map[string]string)
		for _, cur := range composition.ByCurrency {
			allocations[cur.Currency] = cur.Value
			names[cur.Currency] = cur.Currency
		}
	}

	// Calculate recommendations
	for key, targetPct := range targetMap {
		currentValue := allocations[key]
		currentPct := 0.0
		if composition.TotalValue > 0 {
			currentPct = (currentValue / composition.TotalValue) * 100
		}

		targetValue := newTotal * (targetPct / 100)
		diff := targetValue - currentValue

		action := "hold"
		if math.Abs(diff) < 100 { // Less than 100 kr difference
			action = "hold"
			diff = 0
		} else if diff > 0 {
			action = "buy"
		} else {
			action = "sell"
		}

		recommendation.Recommendations = append(recommendation.Recommendations, RebalanceAction{
			TargetKey:  key,
			TargetName: names[key],
			Action:     action,
			Amount:     math.Abs(diff),
			CurrentPct: currentPct,
			TargetPct:  targetPct,
		})
	}

	// Sort by action (buy first, then sell, then hold)
	actionOrder := map[string]int{"buy": 1, "sell": 2, "hold": 3}
	sort.Slice(recommendation.Recommendations, func(i, j int) bool {
		return actionOrder[recommendation.Recommendations[i].Action] < actionOrder[recommendation.Recommendations[j].Action]
	})

	// Add tax tips
	recommendation.TaxTips = s.generateTaxTips(composition, newMoney)

	return recommendation, nil
}

// generateTaxTips creates Danish tax optimization tips based on portfolio state.
func (s *PortfolioService) generateTaxTips(composition *PortfolioComposition, newMoney float64) []TaxTip {
	tips := make([]TaxTip, 0)

	// Check for ASK account optimization
	askValue := 0.0
	for _, at := range composition.ByAssetType {
		if at.AssetType == "ask" {
			askValue = at.Value
			break
		}
	}

	if askValue < ASKMaxDeposit {
		remaining := ASKMaxDeposit - askValue
		tips = append(tips, TaxTip{
			Priority:    1,
			Title:       "Max out ASK",
			Description: "Du kan stadig indsætte " + formatDKK(remaining) + " på din ASK (17% skat vs 27-42%)",
			Action:      "Prioriter ASK for nye investeringer",
		})
	}

	// New money recommendation
	if newMoney > 0 {
		tips = append(tips, TaxTip{
			Priority:    2,
			Title:       "Prioriter skatteeffektive konti",
			Description: "Ved nye investeringer: ASK (17%) → Frie midler → Pension",
			Action:      "Placer " + formatDKK(newMoney) + " i ASK først hvis muligt",
		})
	}

	// Concentration warning
	if composition.ConcentrationPct > 50 {
		tips = append(tips, TaxTip{
			Priority:    3,
			Title:       "Høj koncentration",
			Description: "Top 5 beholdninger udgør " + formatPct(composition.ConcentrationPct) + " - overvej diversificering",
		})
	}

	// Check for unrealized losses (tax-loss harvesting opportunity)
	hasLosses := false
	for _, h := range composition.Holdings {
		if h.ProfitLoss < -1000 { // More than 1000 kr loss
			hasLosses = true
			break
		}
	}
	if hasLosses {
		tips = append(tips, TaxTip{
			Priority:    4,
			Title:       "Skattetab harvesting",
			Description: "Du har urealiserede tab - overvej at realisere for at modregne i fremtidige gevinster",
			Action:      "Sælg tabsgivende positioner og genkøb lignende aktiver",
		})
	}

	// Sort by priority
	sort.Slice(tips, func(i, j int) bool {
		return tips[i].Priority < tips[j].Priority
	})

	return tips
}

// Helper functions
func formatDKK(amount float64) string {
	return formatNumberDK(amount) + " kr"
}

func formatPct(pct float64) string {
	return formatNumberDK(pct) + "%"
}

func formatNumberDK(n float64) string {
	// Format number with Danish locale (dot as thousands separator, comma for decimals)
	intPart := int64(n)

	// Format integer part with thousand separators
	s := strconv.FormatInt(intPart, 10)
	if len(s) <= 3 {
		return s
	}

	// Add dots every 3 digits from the right
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
