package store_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/srunal/srunal-go/internal/seed"
)

// testBotToken is the bot token the signed payloads below are minted with.
const testBotToken = "424242:TESTBOTTOKEN"

// signInitDataFor builds a correctly signed Mini App payload for one Telegram
// chat, exactly like Telegram signs tg.initData.
func signInitDataFor(t *testing.T, chatID int64) string {
	t.Helper()

	values := url.Values{}
	values.Set("query_id", "AAHoperatorTest")
	values.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))
	values.Set("user", fmt.Sprintf(
		`{"id":%d,"first_name":"Оператор","username":"operator_almaty","language_code":"ru"}`, chatID))

	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(testBotToken))
	secretKey := secretMac.Sum(nil)

	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	checkString := ""
	for index, key := range keys {
		if index > 0 {
			checkString += "\n"
		}
		checkString += key + "=" + values.Get(key)
	}

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(checkString))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return values.Encode()
}

// TestOperatorPanelFromInitData checks the operator Mini App gate: a validated
// initData payload of a chat bound to an operator opens the panel without any
// password or login code, while a foreign chat is refused.
func TestOperatorPanelFromInitData(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)
	svc.Cfg.Bot.Token = testBotToken

	operatorChat := int64(100200300)
	if err := svc.Store.BindOperatorChat(ctx, seed.DemoOperatorID, &operatorChat); err != nil {
		t.Fatalf("bind operator chat: %v", err)
	}

	session, err := svc.AdminSessionFromInitData(ctx, signInitDataFor(t, operatorChat))
	if err != nil {
		t.Fatalf("AdminSessionFromInitData: %v", err)
	}
	if session.OperatorID != seed.DemoOperatorID {
		t.Fatalf("operator = %q, want %q", session.OperatorID, seed.DemoOperatorID)
	}

	// A chat that is not bound to an operator must not open the panel.
	const foreignChat = int64(777000111)
	if _, err := svc.AdminSessionFromInitData(ctx, signInitDataFor(t, foreignChat)); err == nil {
		t.Fatal("a foreign chat opened the operator panel")
	}

	// A tampered payload is refused as well (the hash no longer matches).
	tampered := signInitDataFor(t, operatorChat) + "0"
	if _, err := svc.AdminSessionFromInitData(ctx, tampered); err == nil {
		t.Fatal("a tampered payload opened the operator panel")
	}

	// /api/auth/me answers the operator identity so the app can show the entry.
	if id, name, ok := svc.OperatorForChat(ctx, operatorChat); !ok || id != seed.DemoOperatorID || name == "" {
		t.Fatalf("OperatorForChat(bound) = %q %q %v", id, name, ok)
	}
	if _, _, ok := svc.OperatorForChat(ctx, foreignChat); ok {
		t.Fatal("a foreign chat is reported as an operator")
	}
}
