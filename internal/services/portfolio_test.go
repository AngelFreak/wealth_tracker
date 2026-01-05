package services

import (
	"testing"
)

func TestFormatDKK(t *testing.T) {
	tests := []struct {
		amount   float64
		expected string
	}{
		{0, "0 kr"},
		{100, "100 kr"},
		{1000, "1.000 kr"},
		{10000, "10.000 kr"},
		{100000, "100.000 kr"},
		{1000000, "1.000.000 kr"},
		{1234567, "1.234.567 kr"},
	}

	for _, tc := range tests {
		got := formatDKK(tc.amount)
		if got != tc.expected {
			t.Errorf("formatDKK(%f) = %s; want %s", tc.amount, got, tc.expected)
		}
	}
}

func TestFormatNumberDK(t *testing.T) {
	tests := []struct {
		num      float64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{12, "12"},
		{123, "123"},
		{1234, "1.234"},
		{12345, "12.345"},
		{123456, "123.456"},
		{1234567, "1.234.567"},
		{12345678, "12.345.678"},
		{-1234, "-1.234"},
	}

	for _, tc := range tests {
		got := formatNumberDK(tc.num)
		if got != tc.expected {
			t.Errorf("formatNumberDK(%f) = %s; want %s", tc.num, got, tc.expected)
		}
	}
}

func TestInferAssetTypeFromCategory(t *testing.T) {
	tests := []struct {
		categoryName string
		expected     string
	}{
		{"", "other"},
		{"Uncategorized", "other"},
		{"Stocks", "Stocks"},
		{"Bonds", "Bonds"},
		{"Cash", "Cash"},
		{"Real Estate", "Real Estate"},
	}

	for _, tc := range tests {
		got := inferAssetTypeFromCategory(tc.categoryName)
		if got != tc.expected {
			t.Errorf("inferAssetTypeFromCategory(%q) = %q; want %q", tc.categoryName, got, tc.expected)
		}
	}
}

func TestInferSymbolFromCategory(t *testing.T) {
	got := inferSymbolFromCategory("Stocks")
	expected := "ACCOUNT"
	if got != expected {
		t.Errorf("inferSymbolFromCategory(%q) = %q; want %q", "Stocks", got, expected)
	}
}

func TestMinFunc(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 1, -1},
		{100, 50, 50},
	}

	for _, tc := range tests {
		got := min(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("min(%d, %d) = %d; want %d", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestConvertToBase(t *testing.T) {
	// Test without currency service (should return original amount)
	ps := &PortfolioService{baseCurrency: "DKK"}

	amount := 100.0
	got := ps.convertToBase(amount, "USD")
	if got != amount {
		t.Errorf("convertToBase without service: got %f; want %f", got, amount)
	}

	// Test same currency (should return original amount)
	got = ps.convertToBase(amount, "DKK")
	if got != amount {
		t.Errorf("convertToBase same currency: got %f; want %f", got, amount)
	}

	// Test empty currency (should return original amount)
	got = ps.convertToBase(amount, "")
	if got != amount {
		t.Errorf("convertToBase empty currency: got %f; want %f", got, amount)
	}
}

func TestDanishTaxConstants(t *testing.T) {
	// Verify tax constants are reasonable
	if ASKMaxDeposit <= 0 {
		t.Error("ASKMaxDeposit should be positive")
	}
	if StockGainThreshold <= 0 {
		t.Error("StockGainThreshold should be positive")
	}
	if StockGainLowRate <= 0 || StockGainLowRate >= 100 {
		t.Error("StockGainLowRate should be between 0 and 100")
	}
	if StockGainHighRate <= StockGainLowRate {
		t.Error("StockGainHighRate should be greater than StockGainLowRate")
	}
	if ASKTaxRate <= 0 || ASKTaxRate >= 100 {
		t.Error("ASKTaxRate should be between 0 and 100")
	}
}

func TestGenerateTaxTips(t *testing.T) {
	ps := &PortfolioService{baseCurrency: "DKK"}

	// Test with empty portfolio
	composition := &PortfolioComposition{
		TotalValue:       0,
		ByAssetType:      []AssetTypeAllocation{},
		Holdings:         []HoldingAllocation{},
		ConcentrationPct: 0,
	}

	tips := ps.generateTaxTips(composition, 0)
	// Should have at least ASK tip since askValue < ASKMaxDeposit
	found := false
	for _, tip := range tips {
		if tip.Title == "Max out ASK" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'Max out ASK' tip for empty portfolio")
	}

	// Test with new money
	tips = ps.generateTaxTips(composition, 10000)
	found = false
	for _, tip := range tips {
		if tip.Title == "Prioriter skatteeffektive konti" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected tax-efficient account tip when adding new money")
	}

	// Test with high concentration
	composition.ConcentrationPct = 60
	tips = ps.generateTaxTips(composition, 0)
	found = false
	for _, tip := range tips {
		if tip.Title == "Høj koncentration" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected concentration warning when ConcentrationPct > 50")
	}

	// Test with unrealized losses
	composition.Holdings = []HoldingAllocation{
		{Symbol: "TEST", Value: 10000, ProfitLoss: -2000},
	}
	tips = ps.generateTaxTips(composition, 0)
	found = false
	for _, tip := range tips {
		if tip.Title == "Skattetab harvesting" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected tax-loss harvesting tip when there are significant losses")
	}

	// Test tips are sorted by priority
	if len(tips) > 1 {
		for i := 1; i < len(tips); i++ {
			if tips[i].Priority < tips[i-1].Priority {
				t.Error("Tax tips should be sorted by priority")
				break
			}
		}
	}
}

func TestPortfolioCompositionDefaults(t *testing.T) {
	composition := &PortfolioComposition{}

	if composition.TotalValue != 0 {
		t.Error("Default TotalValue should be 0")
	}
	if composition.TotalPositions != 0 {
		t.Error("Default TotalPositions should be 0")
	}
	if composition.ConcentrationPct != 0 {
		t.Error("Default ConcentrationPct should be 0")
	}
}

func TestRebalanceActionOrder(t *testing.T) {
	actionOrder := map[string]int{"buy": 1, "sell": 2, "hold": 3}

	if actionOrder["buy"] >= actionOrder["sell"] {
		t.Error("buy should come before sell")
	}
	if actionOrder["sell"] >= actionOrder["hold"] {
		t.Error("sell should come before hold")
	}
}
