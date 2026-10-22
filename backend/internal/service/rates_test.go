package service

import (
	"testing"
)

// TestParseNBKRate checks the National Bank feed parsing: the tenge value of one
// US dollar lives in <description>, divided by <quant>.
func TestParseNBKRate(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
  <title>Official rates</title>
  <item>
    <title>EUR</title>
    <pubDate>18.10.2026</pubDate>
    <description>491.30</description>
    <quant>1</quant>
  </item>
  <item>
    <title>USD</title>
    <pubDate>18.10.2026</pubDate>
    <description>452.12</description>
    <quant>1</quant>
  </item>
</channel></rss>`)

	rate, err := parseNBKRate(body)
	if err != nil {
		t.Fatalf("parseNBKRate: %v", err)
	}
	if rate != 452.12 {
		t.Fatalf("rate = %v, want 452.12", rate)
	}

	// A feed without USD is an error, not a silent zero.
	if _, err := parseNBKRate([]byte(`<rss><channel><item><title>EUR</title><description>491.3</description><quant>1</quant></item></channel></rss>`)); err == nil {
		t.Fatal("a feed without USD must fail")
	}
}

// TestParseOpenERRate checks the fallback JSON source.
func TestParseOpenERRate(t *testing.T) {
	rate, err := parseOpenERRate([]byte(`{"result":"success","rates":{"KZT":452.25,"RUB":78.1}}`))
	if err != nil {
		t.Fatalf("parseOpenERRate: %v", err)
	}
	if rate != 452.25 {
		t.Fatalf("rate = %v, want 452.25", rate)
	}
	if _, err := parseOpenERRate([]byte(`{"result":"success","rates":{"RUB":78.1}}`)); err == nil {
		t.Fatal("a payload without KZT must fail")
	}
}
