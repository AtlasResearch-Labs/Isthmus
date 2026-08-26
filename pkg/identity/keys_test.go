package identity

import (
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if kp.PrivateKey.IsZero() {
		t.Fatal("private key is all zeroes")
	}

	if kp.PublicKey.IsZero() {
		t.Fatal("public key is all zeroes")
	}

	b64Priv := kp.PrivateKey.String()
	parsedPriv, err := ParseKey(b64Priv)
	if err != nil {
		t.Fatalf("ParseKey failed on base64 private key: %v", err)
	}
	if parsedPriv != kp.PrivateKey {
		t.Fatal("parsed private key does not match original")
	}

	b64Pub := kp.PublicKey.String()
	parsedPub, err := ParseKey(b64Pub)
	if err != nil {
		t.Fatalf("ParseKey failed on base64 public key: %v", err)
	}
	if parsedPub != kp.PublicKey {
		t.Fatal("parsed public key does not match original")
	}

	deviceID := DeviceIDFromPublicKey(kp.PublicKey)
	if len(deviceID) != 32 {
		t.Fatalf("expected device ID length 32 hex chars, got %d", len(deviceID))
	}
}
