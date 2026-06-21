// Package auth holds password hashing helpers shared by the HTTP handlers and
// the database bootstrap. It uses salted PBKDF2-HMAC-SHA256 and verifies in
// constant time, while still accepting the legacy unsalted SHA-256 format so
// existing databases keep working until the next successful login upgrades them.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	// pbkdf2Iterations follows the OWASP 2023 guidance for PBKDF2-HMAC-SHA256.
	pbkdf2Iterations = 600_000
	pbkdf2KeyLen     = 32
	saltLen          = 16

	pbkdf2Prefix = "pbkdf2_sha256$"
	legacyPrefix = "sha256:"
)

// Hash returns a salted PBKDF2 encoding of the password, formatted as
// "pbkdf2_sha256$<iterations>$<salt>$<key>" with base64 (raw) fields.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}
	return fmt.Sprintf("%s%d$%s$%s",
		pbkdf2Prefix,
		pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify reports whether password matches the stored encoding. It accepts both
// the PBKDF2 format and the legacy unsalted SHA-256 format.
func Verify(encoded, password string) bool {
	switch {
	case strings.HasPrefix(encoded, pbkdf2Prefix):
		return verifyPBKDF2(encoded, password)
	case strings.HasPrefix(encoded, legacyPrefix):
		return verifyLegacy(encoded, password)
	default:
		return false
	}
}

// NeedsUpgrade reports whether a stored hash uses an outdated scheme and should
// be re-hashed after a successful login.
func NeedsUpgrade(encoded string) bool {
	return !strings.HasPrefix(encoded, pbkdf2Prefix)
}

func verifyPBKDF2(encoded, password string) bool {
	parts := strings.Split(strings.TrimPrefix(encoded, pbkdf2Prefix), "$")
	if len(parts) != 3 {
		return false
	}
	iter, err := strconv.Atoi(parts[0])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func verifyLegacy(encoded, password string) bool {
	sum := sha256.Sum256([]byte("readress:" + password))
	expected := legacyPrefix + base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(expected)) == 1
}
