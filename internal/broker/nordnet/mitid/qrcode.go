package mitid

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// QRManager handles QR code generation and file management for MitID.
type QRManager struct {
	qrDir     string
	mutex     sync.RWMutex
	stopChan  chan struct{}
	isRunning bool
}

// NewQRManager creates a new QR manager for the given directory.
func NewQRManager(qrDir string) *QRManager {
	return &QRManager{
		qrDir: qrDir,
	}
}

// GenerateQRCodePair generates a pair of QR codes from the channel binding value.
// MitID uses two QR codes that alternate, each containing half of the binding value.
func (m *QRManager) GenerateQRCodePair(channelBindingValue string, updateCount int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	log.Printf("[QR Manager] GenerateQRCodePair called: dir=%s, valueLen=%d, updateCount=%d",
		m.qrDir, len(channelBindingValue), updateCount)

	// Ensure directory exists before writing (it may have been cleaned up)
	if err := os.MkdirAll(m.qrDir, 0755); err != nil {
		log.Printf("[QR Manager] Failed to create directory %s: %v", m.qrDir, err)
		return fmt.Errorf("ensuring QR directory: %w", err)
	}

	halfLen := len(channelBindingValue) / 2
	firstHalf := channelBindingValue[:halfLen]
	secondHalf := channelBindingValue[halfLen:]

	// QR code 1 - first half
	qr1Data := QRData{
		Version:     1,
		Part:        1,
		Type:        2,
		HalfData:    firstHalf,
		UpdateCount: updateCount,
	}
	if err := m.writeQRCode(qr1Data, "qr_frame1.png"); err != nil {
		log.Printf("[QR Manager] Failed to write qr_frame1.png: %v", err)
		return fmt.Errorf("writing QR frame 1: %w", err)
	}
	log.Printf("[QR Manager] Successfully wrote qr_frame1.png")

	// QR code 2 - second half
	qr2Data := QRData{
		Version:     1,
		Part:        2,
		Type:        2,
		HalfData:    secondHalf,
		UpdateCount: updateCount,
	}
	if err := m.writeQRCode(qr2Data, "qr_frame2.png"); err != nil {
		log.Printf("[QR Manager] Failed to write qr_frame2.png: %v", err)
		return fmt.Errorf("writing QR frame 2: %w", err)
	}
	log.Printf("[QR Manager] Successfully wrote qr_frame2.png")

	return nil
}

// writeQRCode generates and writes a single QR code to a file.
func (m *QRManager) writeQRCode(data QRData, filename string) error {
	// Encode data as compact JSON (no spaces)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling QR data: %w", err)
	}

	// Generate QR code PNG
	qr, err := qrcode.New(string(jsonData), qrcode.Medium)
	if err != nil {
		return fmt.Errorf("creating QR code: %w", err)
	}

	// Set border to 1 (like Python implementation)
	qr.DisableBorder = false

	// Write to file
	path := filepath.Join(m.qrDir, filename)
	if err := qr.WriteFile(256, path); err != nil {
		return fmt.Errorf("writing QR file: %w", err)
	}

	return nil
}

// SetCurrentFrame sets which QR frame is currently displayed (1 or 2).
func (m *QRManager) SetCurrentFrame(frame int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(m.qrDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(m.qrDir, "current_frame")
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", frame)), 0644)
}

// SetStatus sets the current authentication status.
func (m *QRManager) SetStatus(status string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(m.qrDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(m.qrDir, "status")
	return os.WriteFile(path, []byte(status), 0644)
}

// GetStatus returns the current authentication status.
func (m *QRManager) GetStatus() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	path := filepath.Join(m.qrDir, "status")
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	return string(data)
}

// GetCurrentFrame returns the current QR frame number.
func (m *QRManager) GetCurrentFrame() (int, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	path := filepath.Join(m.qrDir, "current_frame")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	var frame int
	if _, err := fmt.Sscanf(string(data), "%d", &frame); err != nil {
		return 0, err
	}
	return frame, nil
}

// GetQRCodePath returns the path to the specified QR frame image.
func (m *QRManager) GetQRCodePath(frame int) string {
	return filepath.Join(m.qrDir, fmt.Sprintf("qr_frame%d.png", frame))
}

// EnsureDirectory creates the QR directory if it doesn't exist.
func (m *QRManager) EnsureDirectory() error {
	return os.MkdirAll(m.qrDir, 0755)
}

// Cleanup removes all QR files and the directory.
func (m *QRManager) Cleanup() error {
	return os.RemoveAll(m.qrDir)
}

// QRAnimator handles the frame animation for QR display.
type QRAnimator struct {
	manager  *QRManager
	stopChan chan struct{}
	wg       sync.WaitGroup
	running  bool
	mutex    sync.Mutex
}

// NewQRAnimator creates a new QR animator.
func NewQRAnimator(manager *QRManager) *QRAnimator {
	return &QRAnimator{
		manager: manager,
	}
}

// Start begins alternating between QR frames every second.
func (a *QRAnimator) Start() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return
	}

	a.stopChan = make(chan struct{})
	a.running = true
	a.wg.Add(1)

	go func() {
		defer a.wg.Done()
		frame := 1
		for {
			select {
			case <-a.stopChan:
				return
			default:
				a.manager.SetCurrentFrame(frame)
				frame = 3 - frame // Toggle between 1 and 2

				// Wait 1 second or until stopped
				select {
				case <-a.stopChan:
					return
				case <-waitDuration(1):
				}
			}
		}
	}()
}

// Stop stops the frame animation.
func (a *QRAnimator) Stop() {
	a.mutex.Lock()
	if !a.running {
		a.mutex.Unlock()
		return
	}
	close(a.stopChan)
	a.running = false
	a.mutex.Unlock()

	a.wg.Wait()
}

// waitDuration returns a channel that receives after n seconds.
func waitDuration(seconds int) <-chan time.Time {
	return time.After(time.Duration(seconds) * time.Second)
}
