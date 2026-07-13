package aliases

import (
	"encoding/json"
	"os"
	"path/filepath"

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
func Set(name, email string) error {
	m := Load()
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
func Resolve(pattern string) string {
	if email, ok := Load()[pattern]; ok {
		return email
	}
	return pattern
}
