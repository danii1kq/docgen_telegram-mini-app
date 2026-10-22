// Signed links for the generation log.
//
// The operator console lives inside the Mini App (initData identity), but the
// raw log is often wanted in a plain browser tab - where neither initData nor the
// session cookie exists. A short-lived HMAC token (signed with the bot token)
// authorizes exactly one order's log and nothing else.

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// LogLinkToken returns "<expiresUnix>.<signature>" for one order number.
func LogLinkToken(secret, number string, expires time.Time) string {
	signature := logLinkMac(secret, number, expires.Unix())
	return strconv.FormatInt(expires.Unix(), 10) + "." + signature
}

// VerifyLogLinkToken checks the signature and the expiry (constant time).
func VerifyLogLinkToken(secret, number, token string, now time.Time) bool {
	expiresPart, signature, found := strings.Cut(strings.TrimSpace(token), ".")
	if !found {
		return false
	}
	expires, err := strconv.ParseInt(expiresPart, 10, 64)
	if err != nil || now.Unix() > expires {
		return false
	}
	expected := logLinkMac(secret, number, expires)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func logLinkMac(secret, number string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("console-log:" + number + ":" + strconv.FormatInt(expires, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
