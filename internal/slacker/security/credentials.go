package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

// CredentialCrypto handles encryption and decryption of user credentials
type CredentialCrypto struct {
	masterKey []byte
	keyFile   string
	mutex     sync.RWMutex
}

// NewCredentialCrypto creates a new credential crypto instance
func NewCredentialCrypto(keyFile string) *CredentialCrypto {
	return &CredentialCrypto{
		keyFile: keyFile,
	}
}

// Initialize sets up the encryption with a passphrase
func (cc *CredentialCrypto) Initialize(passphrase string) error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// Derive master key from passphrase using Argon2
	salt, err := cc.getOrGenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Use Argon2id for key derivation (more secure than Argon2)
	key := argon2.IDKey([]byte(passphrase), salt, 1, 64*1024, 4, 32)
	cc.masterKey = key

	return nil
}

// getOrGenerateSalt gets an existing salt or generates a new one
func (cc *CredentialCrypto) getOrGenerateSalt() ([]byte, error) {
	saltFile := cc.keyFile + ".salt"

	// Ensure directory exists before creating salt file
	saltDir := filepath.Dir(saltFile)
	if err := os.MkdirAll(saltDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create salt directory: %w", err)
	}

	// Try to read existing salt
	if data, err := os.ReadFile(saltFile); err == nil && len(data) == 16 {
		return data, nil
	}

	// Generate new salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Save salt to file
	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		return nil, fmt.Errorf("failed to save salt: %w", err)
	}

	return salt, nil
}

// EncryptCredentials encrypts a map of credentials
func (cc *CredentialCrypto) EncryptCredentials(creds map[string]string) ([]byte, error) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()

	if cc.masterKey == nil {
		return nil, fmt.Errorf("crypto not initialized")
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshaling credentials: %w", err)
	}

	block, err := aes.NewCipher(cc.masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// DecryptCredentials decrypts credentials
func (cc *CredentialCrypto) DecryptCredentials(encrypted []byte) (map[string]string, error) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()

	if cc.masterKey == nil {
		return nil, fmt.Errorf("crypto not initialized")
	}

	block, err := aes.NewCipher(cc.masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encrypted[:nonceSize], encrypted[nonceSize:]
	data, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting data: %w", err)
	}

	var creds map[string]string
	err = json.Unmarshal(data, &creds)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling credentials: %w", err)
	}

	return creds, nil
}

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

// Initialize initializes the crypto system with a passphrase
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
		os.Remove(tempFile) // Clean up temp file
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

// CreateKeyFileHash creates a hash of the master key for verification
func (cc *CredentialCrypto) CreateKeyFileHash() (string, error) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()

	if cc.masterKey == nil {
		return "", fmt.Errorf("crypto not initialized")
	}

	hash := sha256.Sum256(cc.masterKey)
	return fmt.Sprintf("%x", hash), nil
}
