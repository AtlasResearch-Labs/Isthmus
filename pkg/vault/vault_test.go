package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedVault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_vault_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	vm := NewManager(tempDir)

	pass := "super-secret-master-passphrase-123"
	payload := []byte("CONFIDENTIAL API KEYS: sk-live-992384910283019283")

	// 1. Test Encrypt & Decrypt Bytes
	encrypted, err := vm.EncryptBytes(payload, pass)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if bytes.Equal(encrypted, payload) {
		t.Fatalf("ciphertext matches plaintext")
	}

	decrypted, err := vm.DecryptBytes(encrypted, pass)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, payload) {
		t.Fatalf("expected '%s', got '%s'", string(payload), string(decrypted))
	}

	// 2. Test Invalid Passphrase
	_, err = vm.DecryptBytes(encrypted, "wrong-passphrase")
	if err != ErrInvalidPassphrase {
		t.Fatalf("expected ErrInvalidPassphrase, got: %v", err)
	}

	// 3. Test File Encrypt & Decrypt
	srcFile := filepath.Join(tempDir, "secrets.txt")
	encFile := filepath.Join(tempDir, "Vault", "secrets.txt.enc")
	decFile := filepath.Join(tempDir, "restored.txt")

	if err := os.WriteFile(srcFile, payload, 0644); err != nil {
		t.Fatalf("write src file err: %v", err)
	}

	if err := vm.EncryptFile(srcFile, encFile, pass); err != nil {
		t.Fatalf("encrypt file err: %v", err)
	}

	if err := vm.DecryptFile(encFile, decFile, pass); err != nil {
		t.Fatalf("decrypt file err: %v", err)
	}

	restoredData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("read restored file err: %v", err)
	}
	if !bytes.Equal(restoredData, payload) {
		t.Fatalf("file content mismatch")
	}

	// 4. Test Lock / Unlock State
	if err := vm.Unlock(pass, 15); err != nil {
		t.Fatalf("unlock err: %v", err)
	}
	if !vm.IsUnlocked() {
		t.Fatalf("expected vault to be unlocked")
	}

	st := vm.Status()
	if !st.Unlocked || st.EncryptedFiles != 1 {
		t.Fatalf("unexpected vault status: %+v", st)
	}

	vm.Lock()
	if vm.IsUnlocked() {
		t.Fatalf("expected vault to be locked after Lock()")
	}
}
