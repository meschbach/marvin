package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPassphraseFromFile(t *testing.T) {
	t.Run("valid passphrase file", func(t *testing.T) {
		// Create temporary file with passphrase
		tmpDir := t.TempDir()
		passFile := filepath.Join(tmpDir, "pass.txt")
		passphrase := "test-secret-123"

		err := os.WriteFile(passFile, []byte(passphrase), 0600)
		require.NoError(t, err)

		// Read passphrase
		result, err := readPassphraseFromFile(passFile)
		require.NoError(t, err)
		assert.Equal(t, passphrase, result)
	})

	t.Run("passphrase file with whitespace", func(t *testing.T) {
		tmpDir := t.TempDir()
		passFile := filepath.Join(tmpDir, "pass.txt")
		passphrase := "test-secret-123\n"

		err := os.WriteFile(passFile, []byte(passphrase), 0600)
		require.NoError(t, err)

		result, err := readPassphraseFromFile(passFile)
		require.NoError(t, err)
		assert.Equal(t, "test-secret-123", result)
	})

	t.Run("missing passphrase file", func(t *testing.T) {
		_, err := readPassphraseFromFile("/nonexistent/pass.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reading passphrase file")
	})

	t.Run("empty passphrase file", func(t *testing.T) {
		tmpDir := t.TempDir()
		passFile := filepath.Join(tmpDir, "empty.txt")

		err := os.WriteFile(passFile, []byte(""), 0600)
		require.NoError(t, err)

		result, err := readPassphraseFromFile(passFile)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("permissive permissions warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		passFile := filepath.Join(tmpDir, "permissive.txt")
		passphrase := "test-secret-123"

		// Create file with permissive permissions (readable by others)
		err := os.WriteFile(passFile, []byte(passphrase), 0644)
		require.NoError(t, err)

		// This should work but show a warning (we can't easily test the warning output)
		result, err := readPassphraseFromFile(passFile)
		require.NoError(t, err)
		assert.Equal(t, passphrase, result)
	})
}
