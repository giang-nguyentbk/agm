package proto

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// CreateUnifiedOAuthToken builds the base64 unified OAuth state entry used by the IDE.
// Matches src/shared/serialization/protobuf.ts createUnifiedOAuthToken.
func CreateUnifiedOAuthToken(accessToken, refreshToken string, expirySeconds int64) string {
	oauthInfo := createOAuthInfo(accessToken, refreshToken, expirySeconds)
	return CreateUnifiedStateEntry("oauthTokenInfoSentinelKey", oauthInfo)
}

// CreateUnifiedStateEntry wraps payload as a unified topic entry and base64-encodes it.
func CreateUnifiedStateEntry(sentinelKey string, payload []byte) string {
	topic := createUnifiedTopicEntry(sentinelKey, payload)
	return base64.StdEncoding.EncodeToString(topic)
}

func createOAuthInfo(accessToken, refreshToken string, expirySeconds int64) []byte {
	f1 := encodeStringField(1, accessToken)
	f2 := encodeStringField(2, "Bearer")
	f3 := encodeStringField(3, refreshToken)

	// Timestamp: field 1 = seconds, field 2 = nanos(0)
	tsInner := append(encodeVarintField(1, uint64(expirySeconds)), encodeVarintField(2, 0)...)
	f4 := encodeLenDelimField(4, tsInner)

	out := make([]byte, 0, len(f1)+len(f2)+len(f3)+len(f4))
	out = append(out, f1...)
	out = append(out, f2...)
	out = append(out, f3...)
	out = append(out, f4...)
	return out
}

func createUnifiedTopicEntry(sentinelKey string, payload []byte) []byte {
	row := encodeStringField(1, base64.StdEncoding.EncodeToString(payload))
	dataEntry := append(encodeStringField(1, sentinelKey), encodeLenDelimField(2, row)...)
	return encodeLenDelimField(1, dataEntry)
}

func encodeVarint(value uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], value)
	return buf[:n]
}

func encodeVarintField(fieldNum int, value uint64) []byte {
	tag := uint64(fieldNum<<3 | 0)
	return append(encodeVarint(tag), encodeVarint(value)...)
}

func encodeStringField(fieldNum int, value string) []byte {
	return encodeLenDelimField(fieldNum, []byte(value))
}

func encodeLenDelimField(fieldNum int, data []byte) []byte {
	tag := uint64(fieldNum<<3 | 2)
	out := encodeVarint(tag)
	out = append(out, encodeVarint(uint64(len(data)))...)
	out = append(out, data...)
	return out
}

// ExtractOAuthFromUnified parses a base64 unified OAuth state entry.
func ExtractOAuthFromUnified(b64 string) (access, refresh string, expiry int64, err error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", "", 0, err
	}
	// Outer field 1 length-delimited
	inner, ok := getBytesField(raw, 1)
	if !ok {
		return "", "", 0, fmt.Errorf("missing outer field 1")
	}
	// Field 2 contains nested row with field 1 = base64(oauthInfo)
	row, ok := getBytesField(inner, 2)
	if !ok {
		return "", "", 0, fmt.Errorf("missing inner field 2")
	}
	oauthB64, ok := getStringField(row, 1)
	if !ok {
		return "", "", 0, fmt.Errorf("missing oauth payload")
	}
	oauthInfo, err := base64.StdEncoding.DecodeString(oauthB64)
	if err != nil {
		// Sometimes payload is raw bytes already
		oauthInfo = []byte(oauthB64)
	}
	access, _ = getStringField(oauthInfo, 1)
	refresh, _ = getStringField(oauthInfo, 3)
	if access == "" || refresh == "" {
		return "", "", 0, fmt.Errorf("incomplete oauth info")
	}
	// Field 4 = Timestamp message
	if ts, ok := getBytesField(oauthInfo, 4); ok {
		if sec, ok := getVarintField(ts, 1); ok {
			expiry = int64(sec)
		}
	}
	return access, refresh, expiry, nil
}

func getBytesField(data []byte, fieldNum int) ([]byte, bool) {
	offset := 0
	for offset < len(data) {
		tag, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return nil, false
		}
		offset += n
		num := int(tag >> 3)
		wire := int(tag & 7)
		switch wire {
		case 0: // varint
			_, n2 := binary.Uvarint(data[offset:])
			if n2 <= 0 {
				return nil, false
			}
			offset += n2
		case 2: // len-delim
			l, n2 := binary.Uvarint(data[offset:])
			if n2 <= 0 {
				return nil, false
			}
			offset += n2
			end := offset + int(l)
			if end > len(data) {
				return nil, false
			}
			if num == fieldNum {
				return data[offset:end], true
			}
			offset = end
		default:
			return nil, false
		}
	}
	return nil, false
}

func getStringField(data []byte, fieldNum int) (string, bool) {
	b, ok := getBytesField(data, fieldNum)
	if !ok {
		return "", false
	}
	return string(b), true
}

func getVarintField(data []byte, fieldNum int) (uint64, bool) {
	offset := 0
	for offset < len(data) {
		tag, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return 0, false
		}
		offset += n
		num := int(tag >> 3)
		wire := int(tag & 7)
		switch wire {
		case 0:
			v, n2 := binary.Uvarint(data[offset:])
			if n2 <= 0 {
				return 0, false
			}
			offset += n2
			if num == fieldNum {
				return v, true
			}
		case 2:
			l, n2 := binary.Uvarint(data[offset:])
			if n2 <= 0 {
				return 0, false
			}
			offset += n2 + int(l)
		default:
			return 0, false
		}
	}
	return 0, false
}
