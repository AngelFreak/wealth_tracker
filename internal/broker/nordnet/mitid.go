package nordnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// MitID authentication errors.
var (
	ErrMitIDTimeout     = errors.New("MitID authentication timed out - user did not approve in time")
	ErrMitIDNotFound    = errors.New("MitID authentication script not found")
	ErrMitIDFailed      = errors.New("MitID authentication failed")
	ErrPythonNotFound   = errors.New("Python3 not found in PATH")
)

const (
	// MitIDTimeout is how long to wait for user to approve in MitID app.
	MitIDTimeout = 2 * time.Minute
)

// mitidAuthResult represents the JSON output from the Python MitID script.
type mitidAuthResult struct {
	Success bool              `json:"success"`
	JWT     string            `json:"jwt"`
	NTag    string            `json:"ntag"`
	Domain  string            `json:"domain"`
	Cookies map[string]string `json:"cookies"`
	Error   string            `json:"error"`
}

// MitIDSession tracks an active MitID authentication session.
type MitIDSession struct {
	ConnectionID int64
	QRDir        string
	StartedAt    time.Time
}

// ActiveMitIDSessions tracks currently active MitID auth sessions by connection ID.
var (
	activeMitIDSessions = make(map[int64]*MitIDSession)
	mitidSessionsMutex  sync.RWMutex
)

// GetActiveMitIDSession returns the active MitID session for a connection, if any.
func GetActiveMitIDSession(connectionID int64) *MitIDSession {
	mitidSessionsMutex.RLock()
	defer mitidSessionsMutex.RUnlock()
	return activeMitIDSessions[connectionID]
}

// GetQRCodePath returns the path to the current QR code image for a connection.
func GetQRCodePath(connectionID int64) (string, error) {
	session := GetActiveMitIDSession(connectionID)
	if session == nil {
		return "", errors.New("no active MitID session")
	}

	// Read current frame
	frameFile := filepath.Join(session.QRDir, "current_frame")
	frameData, err := os.ReadFile(frameFile)
	if err != nil {
		return "", fmt.Errorf("QR code not ready yet: %w", err)
	}

	frame := string(frameData)
	qrPath := filepath.Join(session.QRDir, fmt.Sprintf("qr_frame%s.png", frame))
	return qrPath, nil
}

// GetMitIDStatus returns the current status of MitID authentication.
func GetMitIDStatus(connectionID int64) string {
	session := GetActiveMitIDSession(connectionID)
	if session == nil {
		return "none"
	}

	statusFile := filepath.Join(session.QRDir, "status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return "initializing"
	}
	return string(data)
}

// AuthenticateWithMitID performs MitID authentication by calling the Python script.
// The user must approve the login in their MitID app within the timeout period.
//
// Parameters:
//   - connectionID: ID of the broker connection (for session tracking)
//   - country: Nordnet country code (dk, se, no, fi)
//   - userID: MitID user identifier
//   - method: Authentication method ("APP" for MitID app, "TOKEN" for hardware token)
//   - scriptDir: Directory containing the mitid_auth.py script
//
// Returns a Session on success or an error if authentication fails.
func AuthenticateWithMitID(connectionID int64, country, userID, method, scriptDir string) (*Session, error) {
	if method == "" {
		method = "APP"
	}

	// Prefer venv Python if it exists (has required dependencies)
	venvPython := filepath.Join(scriptDir, "venv", "bin", "python3")
	var pythonPath string

	// Check if venv python exists
	if _, err := os.Stat(venvPython); err == nil {
		pythonPath = venvPython
	} else {
		// Fall back to system Python
		var lookupErr error
		pythonPath, lookupErr = exec.LookPath("python3")
		if lookupErr != nil {
			pythonPath, lookupErr = exec.LookPath("python")
			if lookupErr != nil {
				return nil, ErrPythonNotFound
			}
		}
	}

	// Build script path
	scriptPath := filepath.Join(scriptDir, "mitid_auth.py")

	// Create QR output directory for this session
	qrDir := filepath.Join(os.TempDir(), fmt.Sprintf("mitid_qr_%d", connectionID))
	os.RemoveAll(qrDir) // Clean up any previous session
	if err := os.MkdirAll(qrDir, 0755); err != nil {
		return nil, fmt.Errorf("creating QR directory: %w", err)
	}

	// Track this session
	mitidSessionsMutex.Lock()
	activeMitIDSessions[connectionID] = &MitIDSession{
		ConnectionID: connectionID,
		QRDir:        qrDir,
		StartedAt:    time.Now(),
	}
	mitidSessionsMutex.Unlock()

	// Clean up session when done
	defer func() {
		mitidSessionsMutex.Lock()
		delete(activeMitIDSessions, connectionID)
		mitidSessionsMutex.Unlock()
		// Keep QR files briefly for final status check, clean up in background
		go func() {
			time.Sleep(5 * time.Second)
			os.RemoveAll(qrDir)
		}()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), MitIDTimeout)
	defer cancel()

	// Build command with QR directory
	cmd := exec.CommandContext(ctx, pythonPath, scriptPath,
		"--country", country,
		"--user", userID,
		"--method", method,
		"--qr-dir", qrDir,
	)

	// Set working directory to script directory (for MitID-BrowserClient imports)
	cmd.Dir = scriptDir

	// Execute and capture output (use CombinedOutput to get both stdout and stderr)
	output, err := cmd.CombinedOutput()

	// Always try to parse the output first - Python script outputs JSON to stdout even on error
	var result mitidAuthResult
	if jsonErr := json.Unmarshal(output, &result); jsonErr == nil {
		// Successfully parsed JSON
		if !result.Success {
			errMsg := result.Error
			if errMsg == "" {
				errMsg = "unknown error"
			}
			return nil, fmt.Errorf("%w: %s", ErrMitIDFailed, errMsg)
		}
		// If we get here with Success=true but err!=nil, something weird happened
		// but we'll continue to process the result
	} else if err != nil {
		// JSON parsing failed and command failed
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMitIDTimeout
		}
		// Return whatever output we got for debugging
		if len(output) > 0 {
			// Try to extract any error message from the output
			outputStr := string(output)
			if len(outputStr) > 500 {
				outputStr = outputStr[:500] + "..."
			}
			return nil, fmt.Errorf("%w: %s", ErrMitIDFailed, outputStr)
		}
		return nil, fmt.Errorf("%w: %v", ErrMitIDFailed, err)
	}

	// Check for command error even if JSON parsed successfully
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMitIDTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrMitIDFailed, err)
	}

	// Create session from result
	session := &Session{
		JWT:       result.JWT,
		NTag:      result.NTag,
		Domain:    result.Domain,
		ExpiresAt: time.Now().Add(24 * time.Hour), // JWT typically valid for 24h
	}

	return session, nil
}

// LoginWithMitID is a convenience method on Client to authenticate using MitID.
func (c *Client) LoginWithMitID(connectionID int64, userID, method, scriptDir string) (*Session, error) {
	return AuthenticateWithMitID(connectionID, c.country, userID, method, scriptDir)
}
