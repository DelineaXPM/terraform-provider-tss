package delinea

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
)

// Define constants for salt length and key length
const saltLength = 16
const keyLength = 32
const iterations = 100000

// stateFileMode keeps both the ciphertext and the decrypted state, which
// contains secret material, readable by the owner only.
const stateFileMode = 0o600

func writeStateFile(filename string, data []byte) (err error) {
	dir := filepath.Dir(filename)
	f, err := os.CreateTemp(dir, "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := f.Name()
	defer func() {
		if f != nil {
			_ = f.Close()
		}
		if tempName != "" {
			_ = os.Remove(tempName)
		}
	}()
	if err = restrictStateFile(tempName, f); err != nil {
		return err
	}
	n, err := f.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	f = nil
	if err = os.Rename(tempName, filename); err != nil {
		return err
	}
	tempName = ""
	return nil
}

func readStateFile(filename, description string) ([]byte, bool, error) {
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read %s: %w", description, err)
	}
	return data, true, nil
}

// EncryptFile encrypts the file content
func EncryptFile(passphrase, stateFile string) error {
	data, exists, err := readStateFile(stateFile, "input file")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	// Generate a random salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %v", err)
	}

	// Derive the encryption key using PBKDF2
	key := pbkdf2.Key([]byte(passphrase), salt, iterations, keyLength, sha256.New)

	// Encrypt the data
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher block: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %v", err)
	}

	// Encrypt the data using GCM
	encryptedData := gcm.Seal(nonce, nonce, data, nil)

	// Prepend the salt to the encrypted data
	finalData := append(salt, encryptedData...)

	// Write the encrypted data to the state file
	err = writeStateFile(stateFile, []byte(base64.StdEncoding.EncodeToString(finalData)))
	if err != nil {
		return fmt.Errorf("failed to write encrypted data to state file: %v", err)
	}
	if err = syncStateDirectory(filepath.Dir(stateFile)); err != nil {
		return fmt.Errorf("failed to make encrypted state file replacement durable: %w", err)
	}
	return nil
}

// DecryptFile decrypts the content of the state file
func DecryptFile(passphrase, stateFile string) error {
	encryptedBase64Data, exists, err := readStateFile(stateFile, "encrypted file")
	if err != nil {
		return err
	}
	if !exists {
		if err := writeStateFile(stateFile, nil); err != nil {
			return fmt.Errorf("failed to create protected state file: %w", err)
		}
		return nil
	}

	// Decode the base64-encoded encrypted data
	encryptedData, err := base64.StdEncoding.DecodeString(string(encryptedBase64Data))
	if err != nil {
		return fmt.Errorf("failed to decode base64 data: %v", err)
	}

	if len(encryptedData) < saltLength {
		return fmt.Errorf("encrypted data is too short (%d bytes) to contain the %d-byte salt; the file is not a state file encrypted by this tool", len(encryptedData), saltLength)
	}

	// Extract the salt and encrypted data
	salt := encryptedData[:saltLength]
	encryptedContent := encryptedData[saltLength:]

	// Derive the decryption key using PBKDF2
	key := pbkdf2.Key([]byte(passphrase), salt, iterations, keyLength, sha256.New)

	// Decrypt the data
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher block: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %v", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedContent) < nonceSize+gcm.Overhead() {
		return fmt.Errorf("encrypted data is too short (%d bytes) to contain the nonce and authentication tag; the file is truncated or was not encrypted by this tool", len(encryptedContent))
	}
	nonce, ciphertext := encryptedContent[:nonceSize], encryptedContent[nonceSize:]

	// Decrypt the data using GCM
	decryptedData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt data: %v", err)
	}

	// Write the decrypted data to the state file
	err = writeStateFile(stateFile, decryptedData)
	if err != nil {
		return fmt.Errorf("failed to write decrypted data to state file: %v", err)
	}
	return nil
}
