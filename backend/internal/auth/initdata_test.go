package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testBotToken = "123456789:AAH-test-token"

// testUserJSON mirrors what Telegram serialises into the "user" field.
func testUserJSON(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(InitDataUser{
		ID:           42,
		Username:     "anuarbek",
		FirstName:    "Ануар",
		LastName:     "Бекболатов",
		LanguageCode: "ru",
	})
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	return string(data)
}

// signInitData builds a Telegram-shaped payload and signs it exactly like a
// Mini App client does (an independent implementation of the spec, so the
// production validator is not tested against its own helper).
//
//	hoursAgo           - shifts auth_date into the past
//	withSignature      - includes the Bot API 7.0 "signature" field
//	signOverSignature  - puts signature into the signed check string (older
//	                     clients); otherwise it is excluded (current clients)
func signInitData(t *testing.T, hoursAgo int, withSignature, signOverSignature bool) string {
	t.Helper()

	values := url.Values{}
	values.Set("query_id", "AAHtestQuery")
	values.Set("user", testUserJSON(t))
	values.Set("auth_date", strconv.FormatInt(time.Now().Add(-time.Duration(hoursAgo)*time.Hour).Unix(), 10))
	if withSignature {
		values.Set("signature", "test-signature-not-a-real-ed25519")
	}

	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(testBotToken))
	secretKey := secretMac.Sum(nil)

	excludeSignature := !signOverSignature
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" || (excludeSignature && key == "signature") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))

	return values.Encode()
}

func TestValidateInitDataValid(t *testing.T) {
	raw := signInitData(t, 0, false, false)

	data, err := ValidateInitData(raw, testBotToken, DefaultInitDataTTL)
	if err != nil {
		t.Fatalf("ValidateInitData: %v", err)
	}
	if data.User.ID != 42 {
		t.Fatalf("user id = %d, want 42", data.User.ID)
	}
	if data.User.Username != "anuarbek" {
		t.Fatalf("username = %q, want anuarbek", data.User.Username)
	}
	if data.QueryID != "AAHtestQuery" {
		t.Fatalf("query id = %q, want AAHtestQuery", data.QueryID)
	}
	if data.AuthDate.IsZero() {
		t.Fatal("auth date is zero")
	}
}

func TestValidateInitDataSignatureVariants(t *testing.T) {
	for _, signOver := range []bool{false, true} {
		raw := signInitData(t, 0, true, signOver)
		if _, err := ValidateInitData(raw, testBotToken, DefaultInitDataTTL); err != nil {
			t.Fatalf("signOverSignature=%v: %v", signOver, err)
		}
	}
}

func TestValidateInitDataRejectsTampering(t *testing.T) {
	raw := signInitData(t, 0, false, false)
	tampered := strings.Replace(raw, "anuarbek", "attacker", 1)

	if _, err := ValidateInitData(tampered, testBotToken, DefaultInitDataTTL); !errors.Is(err, ErrInitDataInvalid) {
		t.Fatalf("want ErrInitDataInvalid, got %v", err)
	}
}

func TestValidateInitDataRejectsWrongToken(t *testing.T) {
	raw := signInitData(t, 0, false, false)

	if _, err := ValidateInitData(raw, "999:other-bot-token", DefaultInitDataTTL); !errors.Is(err, ErrInitDataInvalid) {
		t.Fatalf("want ErrInitDataInvalid, got %v", err)
	}
}

func TestValidateInitDataExpired(t *testing.T) {
	raw := signInitData(t, 30, false, false) // 30 hours ago, TTL is 24h

	if _, err := ValidateInitData(raw, testBotToken, DefaultInitDataTTL); !errors.Is(err, ErrInitDataExpired) {
		t.Fatalf("want ErrInitDataExpired, got %v", err)
	}
}

func TestValidateInitDataMissingHash(t *testing.T) {
	values := url.Values{}
	values.Set("user", testUserJSON(t))
	values.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))

	if _, err := ValidateInitData(values.Encode(), testBotToken, DefaultInitDataTTL); !errors.Is(err, ErrInitDataInvalid) {
		t.Fatalf("want ErrInitDataInvalid, got %v", err)
	}
}

func TestValidateInitDataEmptyInputs(t *testing.T) {
	if _, err := ValidateInitData("", testBotToken, DefaultInitDataTTL); !errors.Is(err, ErrInitDataInvalid) {
		t.Fatalf("empty payload: want ErrInitDataInvalid, got %v", err)
	}
	raw := signInitData(t, 0, false, false)
	if _, err := ValidateInitData(raw, "", DefaultInitDataTTL); !errors.Is(err, ErrInitDataInvalid) {
		t.Fatalf("empty token: want ErrInitDataInvalid, got %v", err)
	}
}
