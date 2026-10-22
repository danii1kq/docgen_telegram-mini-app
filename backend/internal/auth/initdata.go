package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mini App init-data validation (core.telegram.org/bots/webapps).
//
// The Mini App sends the raw tg.initData string on every request in the
// X-Telegram-Init-Data header. It is signed with the bot token, so the server
// can trust the chat identity without any login flow:
//
//	data_check_string = sorted "key=value" pairs (except hash/signature)
//	                    joined by "\n"
//	secret_key        = HMAC_SHA256(data = bot_token, key = "WebAppData")
//	hash              = hex(HMAC_SHA256(data = data_check_string, key = secret_key))
//
// Only the bot token can produce a valid signature, so a passing check proves
// the payload was issued by Telegram for this bot.

// ErrInitDataInvalid reports a payload whose signature does not match.
var ErrInitDataInvalid = errors.New("auth: init data signature mismatch")

// ErrInitDataExpired reports a stale auth_date.
var ErrInitDataExpired = errors.New("auth: init data is stale")

// DefaultInitDataTTL bounds how old the payload may be (Telegram itself sees
// initData refreshed on every Mini App launch).
const DefaultInitDataTTL = 24 * time.Hour

// clockSkew allows for clients with a slightly fast clock.
const clockSkew = 5 * time.Minute

// InitDataUser is the "user" JSON payload of initData.
type InitDataUser struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	LanguageCode string `json:"language_code"`
	PhotoURL     string `json:"photo_url"`
}

// InitData is the validated Mini App payload.
type InitData struct {
	User     InitDataUser
	AuthDate time.Time
	QueryID  string
}

// ValidateInitData checks the HMAC signature and the freshness of a raw
// tg.initData string. maxAge <= 0 falls back to DefaultInitDataTTL.
func ValidateInitData(raw, botToken string, maxAge time.Duration) (InitData, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(botToken) == "" {
		return InitData{}, fmt.Errorf("%w: empty payload or token", ErrInitDataInvalid)
	}
	if maxAge <= 0 {
		maxAge = DefaultInitDataTTL
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: malformed payload", ErrInitDataInvalid)
	}

	expected := strings.ToLower(strings.TrimSpace(values.Get("hash")))
	if expected == "" {
		return InitData{}, fmt.Errorf("%w: hash is missing", ErrInitDataInvalid)
	}

	// Bot API 7.0+ adds a "signature" field next to the hash. Different client
	// builds disagreed on whether it participates in the HMAC check, so accept
	// both variants: authenticity is equivalent either way, because forging
	// either HMAC still requires the bot token.
	if !initDataSignatureValid(values, botToken, expected, true) &&
		!initDataSignatureValid(values, botToken, expected, false) {
		return InitData{}, ErrInitDataInvalid
	}

	authDate, err := strconv.ParseInt(strings.TrimSpace(values.Get("auth_date")), 10, 64)
	if err != nil || authDate == 0 {
		return InitData{}, fmt.Errorf("%w: auth_date is missing", ErrInitDataInvalid)
	}
	at := time.Unix(authDate, 0).UTC()
	now := time.Now().UTC()
	if at.Before(now.Add(-maxAge)) || at.After(now.Add(clockSkew)) {
		return InitData{}, ErrInitDataExpired
	}

	// url.ParseQuery keeps the JSON as sent; decoding happens on demand.
	userJSON := values.Get("user")
	if userJSON == "" {
		return InitData{}, fmt.Errorf("%w: user is missing", ErrInitDataInvalid)
	}
	var user InitDataUser
	if err := json.Unmarshal([]byte(userJSON), &user); err != nil || user.ID == 0 {
		return InitData{}, fmt.Errorf("%w: user is malformed", ErrInitDataInvalid)
	}

	return InitData{
		User:     user,
		AuthDate: at,
		QueryID:  values.Get("query_id"),
	}, nil
}

// initDataSignatureValid recomputes the HMAC with the given exclusion set.
func initDataSignatureValid(values url.Values, botToken, expected string, excludeSignature bool) bool {
	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(botToken))
	secretKey := secretMac.Sum(nil)

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(initDataCheckString(values, excludeSignature)))
	computed := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(computed), []byte(expected)) == 1
}

// initDataCheckString sorts the fields alphabetically and joins them as
// "key=value" with "\n", excluding hash and (optionally) signature.
func initDataCheckString(values url.Values, excludeSignature bool) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" || (excludeSignature && key == "signature") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(values.Get(key))
	}
	return builder.String()
}
