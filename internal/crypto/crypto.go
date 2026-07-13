package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/agm/internal/paths"
)

const versionPrefix = "agm_enc_v1:"

// MasterKey is a 32-byte AES-256 key.
type MasterKey []byte

// EnsureMasterKey loads an existing key or creates a new standalone key in the agent dir.
// Standalone keys are plain 64-char hex files at ~/.antigravity-agent/.mk (mode 0600).
func EnsureMasterKey() (MasterKey, error) {
	if key, err := LoadMasterKey(); err == nil {
		return key, nil
	}

	if _, err := paths.EnsureAgentDir(); err != nil {
		return nil, err
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	hexKey := hex.EncodeToString(buf)
	path := paths.MasterKeyPath()
	if err := os.WriteFile(path, []byte(hexKey+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write master key: %w", err)
	}
	return MasterKey(buf), nil
}

// LoadMasterKey resolves the master key (local .mk, keychain, or OSCrypt-encrypted .mk).
func LoadMasterKey() (MasterKey, error) {
	// 1. Plain hex .mk (agent dir preferred via FindMasterKeyPath)
	if mkPath := paths.FindMasterKeyPath(); mkPath != "" {
		raw, err := os.ReadFile(mkPath)
		if err == nil {
			if key, err := parseHexKey(strings.TrimSpace(string(raw))); err == nil {
				return key, nil
			}
			// Chromium OSCrypt / DPAPI blob
			if key, err := decryptSafeStorageKeyFile(raw); err == nil {
				return key, nil
			}
		}
	}

	// 2. System keychain (optional legacy source)
	if key, err := loadKeychainMasterKey(); err == nil && len(key) == 32 {
		return key, nil
	}

	return nil, errors.New("master key not found")
}

func parseHexKey(s string) (MasterKey, error) {
	s = strings.TrimSpace(s)
	if len(s) != 64 || !isHex(s) {
		return nil, errors.New("invalid hex key")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	return MasterKey(b), nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// DecryptValue decrypts an AES-256-GCM DB field (versioned or legacy).
func DecryptValue(key MasterKey, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return value, nil
	}

	payload := value
	if strings.HasPrefix(value, versionPrefix) {
		payload = value[len(versionPrefix):]
	}

	parts := strings.Split(payload, ":")
	if len(parts) != 3 {
		return value, nil
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("iv: %w", err)
	}
	tag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("tag: %w", err)
	}
	ct, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return "", err
	}

	plain, err := gcm.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

// EncryptValue encrypts plaintext to the versioned AES-GCM format.
func EncryptValue(key MasterKey, text string) (string, error) {
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return "", err
	}

	sealed := gcm.Seal(nil, iv, []byte(text), nil)
	if len(sealed) < gcm.Overhead() {
		return "", errors.New("seal produced short output")
	}
	ct := sealed[:len(sealed)-gcm.Overhead()]
	tag := sealed[len(sealed)-gcm.Overhead():]

	return fmt.Sprintf("%s%s:%s:%s",
		versionPrefix,
		hex.EncodeToString(iv),
		hex.EncodeToString(tag),
		hex.EncodeToString(ct),
	), nil
}

func decryptSafeStorageKeyFile(blob []byte) (MasterKey, error) {
	if key, err := dpapiUnprotect(blob); err == nil {
		if mk, err := parseHexKey(strings.TrimSpace(string(key))); err == nil {
			return mk, nil
		}
	}

	if len(blob) > 15 && string(blob[:3]) == "v10" {
		osKey, err := loadOSCryptKey()
		if err != nil {
			return nil, err
		}
		nonce := blob[3:15]
		ciphertext := blob[15:]
		block, err := aes.NewCipher(osKey)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		plain, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, err
		}
		return parseHexKey(strings.TrimSpace(string(plain)))
	}

	return nil, errors.New("unsupported .mk format")
}

func loadOSCryptKey() ([]byte, error) {
	lsPath := paths.FindLocalStatePath()
	if lsPath == "" {
		return nil, errors.New("Local State not found")
	}
	raw, err := os.ReadFile(lsPath)
	if err != nil {
		return nil, err
	}
	var ls struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(raw, &ls); err != nil {
		return nil, err
	}
	if ls.OSCrypt.EncryptedKey == "" {
		return nil, errors.New("os_crypt.encrypted_key missing")
	}

	decoded, err := base64.StdEncoding.DecodeString(ls.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, err
	}
	if len(decoded) > 5 && string(decoded[:5]) == "DPAPI" {
		decoded = decoded[5:]
	}
	return dpapiUnprotect(decoded)
}

// KeySource describes where the key was loaded from (for doctor).
func KeySource() string {
	if p := paths.FindMasterKeyPath(); p != "" {
		return p
	}
	if _, err := loadKeychainMasterKey(); err == nil {
		return "keychain:AntigravityManager/MasterKey"
	}
	return ""
}

// EnsureParentDir is a small helper for tests / callers.
func EnsureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}
