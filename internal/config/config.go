// Package config provides application configuration.
package config

import (
	"os"
	"path/filepath"
)

// Config holds the application configuration.
type Config struct {
	// Server settings
	Port string
	Host string

	// Database settings
	DBPath string

	// Session settings
	SessionSecret string
	SessionMaxAge int // in seconds

	// Broker integration settings
	EncryptionSecret string // Used for encrypting broker credentials

	// Environment
	IsDevelopment bool
}

// New creates a new Config with values from environment variables or defaults.
func New() *Config {
	return &Config{
		Port:             getEnv("PORT", "8080"),
		Host:             getEnv("HOST", "localhost"),
		DBPath:           getEnv("DB_PATH", filepath.Join("data", "wealth.db")),
		SessionSecret:    getEnv("SESSION_SECRET", "change-me-in-production-please"),
		SessionMaxAge:    86400 * 7, // 7 days
		EncryptionSecret: getEnv("ENCRYPTION_SECRET", "change-me-in-production-32chars!"),
		IsDevelopment:    getEnv("ENV", "development") == "development",
	}
}

// Address returns the full address to bind the server to.
func (c *Config) Address() string {
	return c.Host + ":" + c.Port
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
