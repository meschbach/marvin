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

// CreateKeyFileHash creates a hash of master key for verification
func (cc *CredentialCrypto) CreateKeyFileHash() (string, error) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()

	if cc.masterKey == nil {
		return "", fmt.Errorf("crypto not initialized")
	}

	hash := sha256.Sum256(cc.masterKey)
	return fmt.Sprintf("%x", hash), nil
}
