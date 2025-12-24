// Package handlers provides HTTP handlers for the wealth tracker.
package handlers

import "os"

// IsDemoMode returns true if the app is running in demo mode.
func IsDemoMode() bool {
	return os.Getenv("DEMO_MODE") == "true"
}
