package state

import (
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
)

// TabKind identifies the active tab in the UI.
type TabKind int

const (
	TabEvent TabKind = iota
	TabReq
	TabPaste
	TabServe
)

// AppState holds user-configurable state shared across widgets.
type AppState struct {
	mu          sync.RWMutex
	secretInput string
	secretKey   *nostr.SecretKey
	status      string
	tab         TabKind
}

// New creates a ready-to-use state bag.
func New() *AppState {
	return &AppState{tab: TabReq}
}

// SecretInput returns the raw text typed by the user.
func (s *AppState) SecretInput() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secretInput
}

// SecretKey returns the parsed secret key if available.
func (s *AppState) SecretKey() (nostr.SecretKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.secretKey == nil {
		return nostr.SecretKey{}, false
	}
	return *s.secretKey, true
}

// SetSecretInput persists the raw text and tries to parse it into a nostr secret key.
func (s *AppState) SetSecretInput(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secretInput = strings.TrimSpace(text)
	if s.secretInput == "" {
		s.secretKey = nil
		return nil
	}

	sk, err := parseSecretInput(s.secretInput)
	if err != nil {
		s.secretKey = nil
		return err
	}
	s.secretKey = &sk
	return nil
}

func parseSecretInput(text string) (nostr.SecretKey, error) {
	if strings.HasPrefix(text, "nsec1") {
		_, data, err := nip19.Decode(text)
		if err != nil {
			return nostr.SecretKey{}, err
		}
		if sk, ok := data.(nostr.SecretKey); ok {
			return sk, nil
		}
		return nostr.SecretKey{}, errors.New("decoded value is not a secret key")
	}

	var sk nostr.SecretKey
	bytes, err := hex.DecodeString(text)
	if err != nil {
		return nostr.SecretKey{}, err
	}
	if len(bytes) != len(sk) {
		return nostr.SecretKey{}, errors.New("secret key must be 32 bytes")
	}
	copy(sk[:], bytes)
	return sk, nil
}

// Status returns the current status line text.
func (s *AppState) Status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus updates the status line text.
func (s *AppState) SetStatus(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = text
}

// SelectedTab returns the active tab identifier.
func (s *AppState) SelectedTab() TabKind {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tab
}

// SetSelectedTab switches the active tab.
func (s *AppState) SetSelectedTab(tab TabKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tab = tab
}
