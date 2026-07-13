package proto

import (
	"encoding/base64"
	"testing"
)

func TestCreateUnifiedOAuthTokenRoundTripShape(t *testing.T) {
	b64 := CreateUnifiedOAuthToken("access-token-abc", "refresh-token-def", 1700001234)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 {
		t.Fatalf("unexpected short payload: %d", len(raw))
	}
	// Outer should be field 1 length-delimited: tag = (1<<3)|2 = 10
	if raw[0] != 10 {
		t.Fatalf("expected outer field1 tag 0x0a, got 0x%02x", raw[0])
	}
}

func TestCreateUnifiedStateEntry(t *testing.T) {
	entry := CreateUnifiedStateEntry("userStatusSentinelKey", []byte{1, 2, 3, 4, 5})
	raw, err := base64.StdEncoding.DecodeString(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty entry")
	}
}

func TestExtractOAuthFromUnified(t *testing.T) {
	b64 := CreateUnifiedOAuthToken("access-token-abc", "refresh-token-def", 1700001234)
	access, refresh, expiry, err := ExtractOAuthFromUnified(b64)
	if err != nil {
		t.Fatal(err)
	}
	if access != "access-token-abc" || refresh != "refresh-token-def" {
		t.Fatalf("got access=%q refresh=%q", access, refresh)
	}
	if expiry != 1700001234 {
		t.Fatalf("expiry=%d", expiry)
	}
}
