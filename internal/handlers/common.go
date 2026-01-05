// Package handlers provides HTTP handlers for the wealth tracker.
package handlers

import "wealth_tracker/internal/config"

// IsDemoMode returns true if the app is running in demo mode.
// Deprecated: Use config.IsDemoMode() directly.
func IsDemoMode() bool {
	return config.IsDemoMode()
}
