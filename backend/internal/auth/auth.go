// Package auth holds password hashing (Argon2id), human-typable tokens and
// constant-time comparison. The operator panel signs in through the Telegram
// bot (see internal/tgbot), so no TOTP/2FA code lives here.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ---------------------------------------------------------------------------
// Passwords (Argon2id, RFC 9106 / OWASP recommended parameters)
// ---------------------------------------------------------------------------

const (
	argonMemory  = 64 * 1024 // KiB
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns a PHC-formatted Argon2id hash:
// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("auth: empty password")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword compares a password with a stored PHC hash in constant time.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// tokenAlphabet is the unambiguous alphabet from the README (no 0/O/1/I).
const tokenAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// AccessToken returns a 6-character, human-typable order access token.
func AccessToken() (string, error) {
	out := make([]byte, 6)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("auth: token: %w", err)
		}
		out[i] = tokenAlphabet[n.Int64()]
	}
	return string(out), nil
}

// SessionToken returns a 256-bit hex bearer token for a web session.
func SessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NumericCode returns a fixed-length numeric code (the Telegram login code).
func NumericCode(digits int) (string, error) {
	out := make([]byte, digits)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("auth: numeric code: %w", err)
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out), nil
}

// HashToken stores a peppered SHA-256 of a bearer secret. The README suggests
// hashing the order token so lookups cannot be enumerated from the database.
func HashToken(pepper, token string) string {
	sum := sha256.Sum256([]byte(pepper + ":" + token))
	return hex.EncodeToString(sum[:])
}

// Equal compares two secrets in constant time.
func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
