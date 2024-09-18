package delinea

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

// Define constants for salt length and key length
const saltLength = 16
const keyLength = 32
const iterations = 100000

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

// EncryptFile encrypts the file content
func EncryptFile(passphrase, stateFile string) error {
	if !fileExists(stateFile) {
		return nil
	}

	// Read the input file
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %v", err)
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
	err = os.WriteFile(stateFile, []byte(base64.StdEncoding.EncodeToString(finalData)), 0644)
	if err != nil {
		return fmt.Errorf("failed to write encrypted data to state file: %v", err)
	}

	log.Printf("[DEBUG] File encrypted successfully: %s\n", stateFile)
	return nil
}

// DecryptFile decrypts the content of the state file
func DecryptFile(passphrase, stateFile string) error {
	if !fileExists(stateFile) {
		return nil
	}

	// Read the encrypted file
	encryptedBase64Data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %v", err)
	}

	// Decode the base64-encoded encrypted data
	encryptedData, err := base64.StdEncoding.DecodeString(string(encryptedBase64Data))
	if err != nil {
		return fmt.Errorf("failed to decode base64 data: %v", err)
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
	nonce, ciphertext := encryptedContent[:nonceSize], encryptedContent[nonceSize:]

	// Decrypt the data using GCM
	decryptedData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt data: %v", err)
	}

	// Write the decrypted data to the state file
	err = os.WriteFile(stateFile, decryptedData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write decrypted data to state file: %v", err)
	}

	log.Printf("[DEBUG] File decrypted successfully: %s\n", stateFile)
	return nil
}
