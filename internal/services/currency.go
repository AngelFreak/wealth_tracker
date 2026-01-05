// Package services provides business logic services.
package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"wealth_tracker/internal/database"
)

// CurrencyRate represents an exchange rate.
type CurrencyRate struct {
	From      string
	To        string
	Rate      float64
	FetchedAt time.Time
}

// CurrencyService provides currency conversion functionality.
type CurrencyService struct {
	db     *database.DB
	cache  map[string]CurrencyRate
	mu     sync.RWMutex
	maxAge time.Duration
}

// NewCurrencyService creates a new CurrencyService.
func NewCurrencyService(db *database.DB) *CurrencyService {
	return &CurrencyService{
		db:     db,
		cache:  make(map[string]CurrencyRate),
		maxAge: 24 * time.Hour, // Rates are cached for 24 hours
	}
}

// Convert converts an amount from one currency to another.
func (s *CurrencyService) Convert(amount float64, from, to string) (float64, error) {
	if from == to {
		return amount, nil
	}

	rate, err := s.GetRate(from, to)
	if err != nil {
		return 0, err
	}

	return amount * rate, nil
}

// GetRate returns the exchange rate from one currency to another.
func (s *CurrencyService) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1.0, nil
	}

	cacheKey := from + "_" + to

	// Check memory cache first
	s.mu.RLock()
	if rate, ok := s.cache[cacheKey]; ok && time.Since(rate.FetchedAt) < s.maxAge {
		s.mu.RUnlock()
		return rate.Rate, nil
	}
	s.mu.RUnlock()

	// Check database cache
	rate, err := s.getFromDB(from, to)
	if err == nil && time.Since(rate.FetchedAt) < s.maxAge {
		s.mu.Lock()
		s.cache[cacheKey] = rate
		s.mu.Unlock()
		return rate.Rate, nil
	}

	// Fetch fresh rate from API
	freshRate, err := s.fetchRate(from, to)
	if err != nil {
		// If API fails, try to use stale rate from DB
		if rate.Rate > 0 {
			log.Printf("Using stale rate for %s/%s due to API error: %v", from, to, err)
			return rate.Rate, nil
		}
		return 0, fmt.Errorf("failed to get exchange rate %s/%s: %w", from, to, err)
	}

	// Store in DB and cache
	if err := s.saveToDB(from, to, freshRate); err != nil {
		log.Printf("Failed to save rate to DB: %v", err)
	}

	s.mu.Lock()
	s.cache[cacheKey] = CurrencyRate{
		From:      from,
		To:        to,
		Rate:      freshRate,
		FetchedAt: time.Now(),
	}
	s.mu.Unlock()

	return freshRate, nil
}

// getFromDB retrieves a rate from the database.
func (s *CurrencyService) getFromDB(from, to string) (CurrencyRate, error) {
	var rate CurrencyRate
	err := s.db.QueryRow(`
		SELECT from_currency, to_currency, rate, fetched_at
		FROM currency_rates
		WHERE from_currency = ? AND to_currency = ?
	`, from, to).Scan(&rate.From, &rate.To, &rate.Rate, &rate.FetchedAt)

	return rate, err
}

// saveToDB stores a rate in the database.
func (s *CurrencyService) saveToDB(from, to string, rate float64) error {
	_, err := s.db.Exec(`
		INSERT INTO currency_rates (from_currency, to_currency, rate, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(from_currency, to_currency)
		DO UPDATE SET rate = excluded.rate, fetched_at = excluded.fetched_at
	`, from, to, rate, time.Now())
	return err
}

// fetchRate fetches a rate from an external API.
// Uses the free exchangerate-api.com service.
func (s *CurrencyService) fetchRate(from, to string) (float64, error) {
	// Using exchangerate-api.com (free tier, no API key needed for basic usage)
	url := fmt.Sprintf("https://api.exchangerate-api.com/v4/latest/%s", from)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("fetching exchange rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("exchange rate API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("parsing exchange rate response: %w", err)
	}

	rate, ok := result.Rates[to]
	if !ok {
		return 0, fmt.Errorf("no rate found for %s/%s", from, to)
	}

	return rate, nil
}

// ConvertToBase converts an amount to a base currency.
func (s *CurrencyService) ConvertToBase(amount float64, from, baseCurrency string) (float64, error) {
	return s.Convert(amount, from, baseCurrency)
}

// GetRatesForCurrencies returns rates for converting multiple currencies to a base currency.
func (s *CurrencyService) GetRatesForCurrencies(currencies []string, baseCurrency string) (map[string]float64, error) {
	rates := make(map[string]float64)

	for _, currency := range currencies {
		rate, err := s.GetRate(currency, baseCurrency)
		if err != nil {
			return nil, fmt.Errorf("getting rate for %s: %w", currency, err)
		}
		rates[currency] = rate
	}

	return rates, nil
}

// RefreshRates refreshes rates for common currency pairs.
func (s *CurrencyService) RefreshRates(baseCurrency string) error {
	commonCurrencies := []string{"USD", "EUR", "GBP", "SEK", "NOK", "DKK", "CHF", "JPY"}

	for _, currency := range commonCurrencies {
		if currency == baseCurrency {
			continue
		}
		if _, err := s.GetRate(currency, baseCurrency); err != nil {
			log.Printf("Failed to refresh rate %s/%s: %v", currency, baseCurrency, err)
		}
	}

	return nil
}

// ClearCache clears the in-memory rate cache.
func (s *CurrencyService) ClearCache() {
	s.mu.Lock()
	s.cache = make(map[string]CurrencyRate)
	s.mu.Unlock()
}
