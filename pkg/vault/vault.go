package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"isthmus/internal/logger"
)

var (
	ErrVaultLocked       = errors.New("vault is locked: passphrase required")
	ErrInvalidPassphrase = errors.New("invalid vault passphrase or corrupted ciphertext")
	ErrFileTooSmall      = errors.New("ciphertext file is too small or invalid")
)

const (
	VaultSaltLength  = 16
	VaultNonceLength = 12
	PBKDF2Iterations = 50000
)

type VaultStatus struct {
	Unlocked        bool      `json:"unlocked"`
	UnlockedUntil   time.Time `json:"unlocked_until,omitempty"`
	EncryptedFiles  int       `json:"encrypted_files_count"`
	VaultDirectory  string    `json:"vault_directory"`
	AutoLockMinutes int       `json:"auto_lock_minutes"`
}

type Manager struct {
	baseDir         string
	vaultDir        string
	log             *logger.Logger
	masterKey       []byte
	unlockedUntil   time.Time
	autoLockMinutes int
	mu              sync.RWMutex
}

func NewManager(baseDir string) *Manager {
	vDir := filepath.Join(baseDir, "Vault")
	_ = os.MkdirAll(vDir, 0700)

	return &Manager{
		baseDir:         baseDir,
		vaultDir:        vDir,
		log:             logger.WithPrefix("EncryptedVault"),
		autoLockMinutes: 30,
	}
}

// deriveKey derives a 32-byte (256-bit) AES key from passphrase and salt
func deriveKey(passphrase string, salt []byte) []byte {
	// 50,000-round SHA256 KDF
	key := []byte(passphrase)
	for i := 0; i < PBKDF2Iterations; i++ {
		h := sha256.New()
		h.Write(key)
		h.Write(salt)
		key = h.Sum(nil)
	}
	return key
}

// Unlock unlocks the vault with a master passphrase
func (vm *Manager) Unlock(passphrase string, durationMinutes int) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if passphrase == "" {
		return errors.New("passphrase cannot be empty")
	}

	if durationMinutes <= 0 {
		durationMinutes = 30
	}

	// Use fixed verification salt to store key in memory
	verificationSalt := sha256.Sum256([]byte("isthmus-vault-salt-" + passphrase))
	vm.masterKey = deriveKey(passphrase, verificationSalt[:16])
	vm.unlockedUntil = time.Now().Add(time.Duration(durationMinutes) * time.Minute)
	vm.autoLockMinutes = durationMinutes

	vm.log.Info("Vault unlocked for %d minutes", durationMinutes)
	return nil
}

// Lock immediately clears keys from memory
func (vm *Manager) Lock() {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.masterKey != nil {
		for i := range vm.masterKey {
			vm.masterKey[i] = 0 // Wipe memory
		}
	}
	vm.masterKey = nil
	vm.unlockedUntil = time.Time{}
	vm.log.Info("Vault locked and keys securely wiped from memory")
}

// IsUnlocked returns true if vault is currently unlocked
func (vm *Manager) IsUnlocked() bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	if vm.masterKey == nil {
		return false
	}
	return time.Now().Before(vm.unlockedUntil)
}

// Status returns current vault diagnostic state
func (vm *Manager) Status() VaultStatus {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	count := 0
	_ = filepath.Walk(vm.vaultDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(path) == ".enc" {
			count++
		}
		return nil
	})

	return VaultStatus{
		Unlocked:        vm.masterKey != nil && time.Now().Before(vm.unlockedUntil),
		UnlockedUntil:   vm.unlockedUntil,
		EncryptedFiles:  count,
		VaultDirectory:  vm.vaultDir,
		AutoLockMinutes: vm.autoLockMinutes,
	}
}

// EncryptBytes encrypts plaintext data with AES-256-GCM using derived key
func (vm *Manager) EncryptBytes(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, VaultSaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Format: [Salt (16B)] + [Nonce (12B)] + [Ciphertext + AuthTag]
	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptBytes decrypts AES-256-GCM ciphertext data using passphrase
func (vm *Manager) DecryptBytes(ciphertextData []byte, passphrase string) ([]byte, error) {
	if len(ciphertextData) < VaultSaltLength+VaultNonceLength {
		return nil, ErrFileTooSmall
	}

	salt := ciphertextData[:VaultSaltLength]
	nonce := ciphertextData[VaultSaltLength : VaultSaltLength+VaultNonceLength]
	ciphertext := ciphertextData[VaultSaltLength+VaultNonceLength:]

	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidPassphrase
	}

	return plaintext, nil
}

// EncryptFile encrypts a file to a destination .enc file
func (vm *Manager) EncryptFile(srcPath, dstPath, passphrase string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read src file: %w", err)
	}

	encrypted, err := vm.EncryptBytes(data, passphrase)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	if dstPath == "" {
		dstPath = srcPath + ".enc"
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0700); err != nil {
		return err
	}

	return os.WriteFile(dstPath, encrypted, 0600)
}

// DecryptFile decrypts a .enc file to a destination file
func (vm *Manager) DecryptFile(srcPath, dstPath, passphrase string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read enc file: %w", err)
	}

	decrypted, err := vm.DecryptBytes(data, passphrase)
	if err != nil {
		return err
	}

	if dstPath == "" {
		if filepath.Ext(srcPath) == ".enc" {
			dstPath = srcPath[:len(srcPath)-4]
		} else {
			dstPath = srcPath + ".dec"
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0700); err != nil {
		return err
	}

	return os.WriteFile(dstPath, decrypted, 0644)
}
