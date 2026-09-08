package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// NewCheckpointID returns a checkpoint/manifest id (cp_…).
func NewCheckpointID() string {
	return NewID("cp")
}

// NewID returns a random prefixed identifier. Used for checkpoint objects.
func NewID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	if prefix == "" {
		prefix = "id"
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}

// NewSessionID returns XXXXXXXX-XXXXXXXX from 8 random bytes (uppercase hex).
func NewSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	h := strings.ToUpper(hex.EncodeToString(b[:]))
	return h[:8] + "-" + h[8:]
}

// CanonicalSessionID validates and uppercases a session id.
func CanonicalSessionID(id string) (string, error) {
	id = strings.TrimSpace(strings.ToUpper(id))
	if !ValidSessionID(id) {
		return "", fmt.Errorf("invalid session id %q (want XXXXXXXX-XXXXXXXX)", id)
	}
	return id, nil
}

// ValidSessionID reports whether id is XXXXXXXX-XXXXXXXX hex.
func ValidSessionID(id string) bool {
	if len(id) != 17 || id[8] != '-' {
		return false
	}
	for i := 0; i < 17; i++ {
		if i == 8 {
			continue
		}
		c := id[i]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

// LooksLikeCheckpointID is the CLI reject path for raw cp_ ids.
func LooksLikeCheckpointID(id string) bool {
	return strings.HasPrefix(strings.ToLower(id), "cp_")
}
