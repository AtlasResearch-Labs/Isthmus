package config

import (
	"testing"
)

func TestValidatePathAccess(t *testing.T) {
	acl := PeerACL{
		AllowRead:    true,
		AllowWrite:   false,
		AllowedPaths: []string{"projects", "shared/public"},
		BlockedPaths: []string{".ssh", ".env", "projects/secret"},
	}

	// 1. Allowed reads
	if err := ValidatePathAccess(acl, "projects/app.go", false); err != nil {
		t.Fatalf("expected allowed read, got err: %v", err)
	}
	if err := ValidatePathAccess(acl, "shared/public/doc.pdf", false); err != nil {
		t.Fatalf("expected allowed read, got err: %v", err)
	}

	// 2. Denied write
	if err := ValidatePathAccess(acl, "projects/app.go", true); err == nil {
		t.Fatal("expected write access error, got nil")
	}

	// 3. Blocked path (including case-insensitive evasion attempts)
	if err := ValidatePathAccess(acl, ".ssh/id_rsa", false); err == nil {
		t.Fatal("expected blocked path error for .ssh, got nil")
	}
	if err := ValidatePathAccess(acl, ".SSH/id_rsa", false); err == nil {
		t.Fatal("expected case-insensitive blocked path error for .SSH, got nil")
	}
	if err := ValidatePathAccess(acl, ".ENV", false); err == nil {
		t.Fatal("expected case-insensitive blocked path error for .ENV, got nil")
	}
	if err := ValidatePathAccess(acl, "projects/secret/keys.json", false); err == nil {
		t.Fatal("expected blocked path error for projects/secret, got nil")
	}

	// 4. Non-whitelisted path
	if err := ValidatePathAccess(acl, "private/data.txt", false); err == nil {
		t.Fatal("expected non-whitelisted path error, got nil")
	}
}
