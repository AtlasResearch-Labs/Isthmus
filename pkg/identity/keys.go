package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
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
	PrivateKey   Key    `json:"private_key"`
	PublicKey    Key    `json:"public_key"`
	SSHPublicKey string `json:"ssh_public_key,omitempty"`
}

func GenerateKeyPair() (*KeyPair, error) {
	var priv Key
	if _, err := io.ReadFull(rand.Reader, priv[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	edPriv := ed25519.NewKeyFromSeed(priv[:])
	var pub Key
	copy(pub[:], edPriv[32:])

	sshPubStr, _ := SSHAuthorizedKeyFromSeed(priv)

	return &KeyPair{
		PrivateKey:   priv,
		PublicKey:    pub,
		SSHPublicKey: sshPubStr,
	}, nil
}

func DeviceIDFromPublicKey(pub Key) string {
	h := sha256.Sum256(pub[:])
	return hex.EncodeToString(h[:16])
}

// SSHSignerFromSeed creates an ssh.Signer derived deterministically from the 32-byte private key.
func SSHSignerFromSeed(privKey Key) (ssh.Signer, error) {
	edPriv := ed25519.NewKeyFromSeed(privKey[:])
	return ssh.NewSignerFromKey(edPriv)
}

// SSHAuthorizedKeyFromSeed returns the OpenSSH formatted public key string for a 32-byte key.
func SSHAuthorizedKeyFromSeed(privKey Key) (string, error) {
	signer, err := SSHSignerFromSeed(privKey)
	if err != nil {
		return "", err
	}
	return string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
}

// SSHAuthorizedKeyFromPubKey returns the OpenSSH formatted public key from an Ed25519 public key.
func SSHAuthorizedKeyFromPubKey(pubKey Key) string {
	edPub := ed25519.PublicKey(pubKey[:])
	sshPub, err := ssh.NewPublicKey(edPub)
	if err != nil {
		return ""
	}
	return string(ssh.MarshalAuthorizedKey(sshPub))
}
