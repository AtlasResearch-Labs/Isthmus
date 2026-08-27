package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type PeerACL struct {
	AllowRead    bool     `json:"allow_read"`
	AllowWrite   bool     `json:"allow_write"`
	AllowedPaths []string `json:"allowed_paths,omitempty"`
	BlockedPaths []string `json:"blocked_paths,omitempty"`
}

func DefaultPeerACL() PeerACL {
	return PeerACL{
		AllowRead:    true,
		AllowWrite:   true,
		AllowedPaths: nil, // All paths permitted under shared root
		BlockedPaths: []string{".ssh", ".git", ".env", "credentials"},
	}
}

func NormalizeSubpath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return ""
	}
	return p
}

func ValidatePathAccess(acl PeerACL, subPath string, isWrite bool) error {
	subPath = NormalizeSubpath(subPath)

	if isWrite && !acl.AllowWrite {
		return errors.New("access denied: peer write permission is disabled")
	}
	if !isWrite && !acl.AllowRead {
		return errors.New("access denied: peer read permission is disabled")
	}

	// Check blocked paths (case-insensitive security hardening)
	for _, blocked := range acl.BlockedPaths {
		normBlocked := NormalizeSubpath(blocked)
		if normBlocked == "" {
			continue
		}
		if strings.EqualFold(subPath, normBlocked) || strings.HasPrefix(strings.ToLower(subPath), strings.ToLower(normBlocked)+"/") {
			return fmt.Errorf("access denied: path '%s' is blocked by security ACL", subPath)
		}
	}

	// If allowed paths whitelist is set, path must match at least one
	if len(acl.AllowedPaths) > 0 {
		allowed := false
		for _, allow := range acl.AllowedPaths {
			normAllow := NormalizeSubpath(allow)
			if normAllow == "" {
				allowed = true
				break
			}
			if strings.EqualFold(subPath, normAllow) ||
				strings.HasPrefix(strings.ToLower(subPath), strings.ToLower(normAllow)+"/") ||
				strings.HasPrefix(strings.ToLower(normAllow), strings.ToLower(subPath)+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("access denied: path '%s' is not in the allowed paths list", subPath)
		}
	}

	return nil
}
