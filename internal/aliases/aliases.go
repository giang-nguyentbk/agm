package aliases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/agm/internal/paths"
)

func path() string {
	return filepath.Join(paths.PrimaryManagerDir(), "aliases.json")
}

// Load returns all aliases.
func Load() map[string]string {
	raw, err := os.ReadFile(path())
	if err != nil {
		return map[string]string{}
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return map[string]string{}
	}
	return m
}

// Save writes aliases to disk.
func Save(m map[string]string) error {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

// Set sets name -> email.
// It auto-detects inverted arguments (e.g. 'email alias' instead of 'alias email')
// and removes any existing reverse mapping to prevent lookup poisoning.
func Set(name, email string) error {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	// Auto-detect inverted arguments: if name contains '@' and email does not, swap them
	if strings.Contains(name, "@") && !strings.Contains(email, "@") {
		name, email = email, name
	}

	m := Load()
	// Clean up any stale reverse mapping that would poison lookups
	delete(m, email)
	m[name] = email
	return Save(m)
}

// Remove deletes an alias.
func Remove(name string) bool {
	m := Load()
	if _, ok := m[name]; !ok {
		return false
	}
	delete(m, name)
	_ = Save(m)
	return true
}

// Resolve maps alias to email, or returns pattern unchanged.
// It guards against corrupted reverse mappings (e.g. an email mapped to a nickname).
func Resolve(pattern string) string {
	m := Load()
	if email, ok := m[pattern]; ok {
		// Guard against corrupted reverse mappings: if pattern is already an email
		// and the mapped value is not an email, do not corrupt the original email.
		if strings.Contains(pattern, "@") && !strings.Contains(email, "@") {
			return pattern
		}
		return email
	}
	return pattern
}
