package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
)

const (
	KeyLength = 32
)

type Key [KeyLength]byte

func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

func (k Key) Hex() string {
	return hex.EncodeToString(k[:])
}

func (k Key) IsZero() bool {
	var zero Key
	return k == zero
}

func ParseKey(s string) (Key, error) {
	var k Key
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = hex.DecodeString(s)
		if err != nil {
			return k, errors.New("invalid key encoding: must be base64 or hex")
		}
	}
	if len(b) != KeyLength {
		return k, fmt.Errorf("invalid key length: got %d bytes, expected %d", len(b), KeyLength)
	}
	copy(k[:], b)
	return k, nil
}

type KeyPair struct {
	PrivateKey Key `json:"private_key"`
	PublicKey  Key `json:"public_key"`
}

func GenerateKeyPair() (*KeyPair, error) {
	var priv Key
	if _, err := io.ReadFull(rand.Reader, priv[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp the private key according to Curve25519 specification
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pubBytes, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive public key: %w", err)
	}

	var pub Key
	copy(pub[:], pubBytes)

	return &KeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}, nil
}

func DeviceIDFromPublicKey(pub Key) string {
	h := sha256.Sum256(pub[:])
	return hex.EncodeToString(h[:16])
}
