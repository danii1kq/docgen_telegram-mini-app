package auth

import (
	"strings"
	"testing"
)

// TestPasswordHashRoundTrip checks Argon2id hashing and verification.
func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("2026")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}
	if !VerifyPassword(hash, "2026") {
		t.Error("correct password rejected")
	}
	if VerifyPassword(hash, "2027") {
		t.Error("wrong password accepted")
	}
	if VerifyPassword("not-a-hash", "2026") {
		t.Error("garbage hash accepted")
	}

	// The same password must produce different hashes (random salt).
	second, err := HashPassword("2026")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if first, second := hash, second; first == second {
		t.Error("salt is not random")
	}
}

// TestAccessToken checks the alphabet and length of order tokens.
func TestAccessToken(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		token, err := AccessToken()
		if err != nil {
			t.Fatalf("AccessToken: %v", err)
		}
		if len(token) != 6 {
			t.Fatalf("token %q has length %d, want 6", token, len(token))
		}
		for _, r := range token {
			if !strings.ContainsRune(tokenAlphabet, r) {
				t.Fatalf("token %q contains %q outside the alphabet", token, r)
			}
			if strings.ContainsRune("01IO", r) {
				t.Fatalf("token %q contains an ambiguous character", token)
			}
		}
		seen[token] = true
	}
	if len(seen) < 190 {
		t.Errorf("tokens are not random enough: %d unique out of 200", len(seen))
	}
}

// TestHashToken checks the peppered hash: deterministic, pepper-sensitive.
func TestHashToken(t *testing.T) {
	first := HashToken("pepper-a", "CLS28G")
	second := HashToken("pepper-a", "CLS28G")
	if first != second {
		t.Error("hash is not deterministic")
	}
	if first == HashToken("pepper-b", "CLS28G") {
		t.Error("hash does not depend on the pepper")
	}
	if len(first) != 64 {
		t.Errorf("hash length = %d, want 64", len(first))
	}
	if !Equal(first, second) || Equal(first, HashToken("pepper-a", "OTHER1")) {
		t.Error("constant-time compare misbehaves")
	}
}

// TestNumericCode checks the Telegram login codes.
func TestNumericCode(t *testing.T) {
	code, err := NumericCode(6)
	if err != nil {
		t.Fatalf("NumericCode: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code %q has length %d", code, len(code))
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Fatalf("code %q is not numeric", code)
		}
	}
}
