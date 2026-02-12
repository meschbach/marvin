package security

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// UserCredentialStore manages per-user encrypted credentials
type UserCredentialStore struct {
	crypto    *CredentialCrypto
	storePath string
	cache     map[string]map[string]string // userID -> credentials
	mutex     sync.RWMutex
}

// NewUserCredentialStore creates a new user credential store
func NewUserCredentialStore(storePath, keyFile string) *UserCredentialStore {
	return &UserCredentialStore{
		crypto:    NewCredentialCrypto(keyFile),
		storePath: storePath,
		cache:     make(map[string]map[string]string),
	}
}

// Initialize initializes crypto system with a passphrase
func (ucs *UserCredentialStore) Initialize(passphrase string) error {
	return ucs.crypto.Initialize(passphrase)
}

// SetUserCredentials stores encrypted credentials for a user
func (ucs *UserCredentialStore) SetUserCredentials(userID string, creds map[string]string) error {
	// Encrypt credentials
	encrypted, err := ucs.crypto.EncryptCredentials(creds)
	if err != nil {
		return fmt.Errorf("encrypting credentials: %w", err)
	}

	// Ensure store directory exists
	if err := os.MkdirAll(ucs.storePath, 0755); err != nil {
		return fmt.Errorf("creating credential store directory: %w", err)
	}

	// Store in encrypted file per user
	filename := filepath.Join(ucs.storePath, fmt.Sprintf("user-%s.enc", userID))

	// Write to temporary file first
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, encrypted, 0600); err != nil {
		return fmt.Errorf("writing temp credential file: %w", err)
	}

	// Rename to final file
	if err := os.Rename(tempFile, filename); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			// Log cleanup error but don't overwrite the main error
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFile, removeErr)
		} // Clean up temp file
		return fmt.Errorf("renaming credential file: %w", err)
	}

	// Update cache
	ucs.mutex.Lock()
	ucs.cache[userID] = creds
	ucs.mutex.Unlock()

	return nil
}

// GetUserCredentials retrieves and decrypts credentials for a user
func (ucs *UserCredentialStore) GetUserCredentials(userID string) (map[string]string, error) {
	// Check cache first
	ucs.mutex.RLock()
	if cached, exists := ucs.cache[userID]; exists {
		ucs.mutex.RUnlock()
		// Return a copy to prevent modification
		credsCopy := make(map[string]string)
		for k, v := range cached {
			credsCopy[k] = v
		}
		return credsCopy, nil
	}
	ucs.mutex.RUnlock()

	filename := filepath.Join(ucs.storePath, fmt.Sprintf("user-%s.enc", userID))
	encrypted, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// No credentials exist, return empty map
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("reading credential file: %w", err)
	}

	creds, err := ucs.crypto.DecryptCredentials(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting credentials: %w", err)
	}

	// Update cache
	ucs.mutex.Lock()
	ucs.cache[userID] = creds
	ucs.mutex.Unlock()

	return creds, nil
}

// DeleteUserCredentials removes credentials for a user
func (ucs *UserCredentialStore) DeleteUserCredentials(userID string) error {
	filename := filepath.Join(ucs.storePath, fmt.Sprintf("user-%s.enc", userID))

	// Remove file
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credential file: %w", err)
	}

	// Remove from cache
	ucs.mutex.Lock()
	delete(ucs.cache, userID)
	ucs.mutex.Unlock()

	return nil
}

// ListUsers returns a list of user IDs that have stored credentials
func (ucs *UserCredentialStore) ListUsers() ([]string, error) {
	files, err := os.ReadDir(ucs.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading credential directory: %w", err)
	}

	var users []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if filename matches pattern
		var userID string
		_, err := fmt.Sscanf(file.Name(), "user-%s.enc", &userID)
		if err == nil && userID != "" {
			users = append(users, userID)
		}
	}

	return users, nil
}
