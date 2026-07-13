package target

import (
	"fmt"
	"strings"
)

// Target is an Antigravity product surface.
type Target string

const (
	Classic Target = "classic" // Antigravity desktop (GUI)
	IDE     Target = "ide"     // Antigravity IDE
	Agy     Target = "agy"     // Antigravity CLI (agy)
	All     Target = "all"     // apply to every relevant surface
)

// Parse normalizes a user-supplied target name.
func Parse(s string) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all", "both", "*":
		return All, nil
	case "classic", "desktop", "gui":
		return Classic, nil
	case "ide":
		return IDE, nil
	case "agy", "cli", "antigravity-cli", "antigravity_cli":
		return Agy, nil
	default:
		return "", fmt.Errorf("unknown target %q (want classic|ide|agy|all)", s)
	}
}

// Expand turns All into concrete targets for injection order.
func Expand(t Target) []Target {
	if t == All {
		// CLI first (no process restart), then IDE, then classic desktop
		return []Target{Agy, IDE, Classic}
	}
	return []Target{t}
}

// UsesCredentialStore reports whether this target writes the OS credential store
// (see CredentialStoreInjectionAdapter.shouldInjectTokenIntoCredentialStore).
func UsesCredentialStore(t Target) bool {
	switch t {
	case Agy, Classic:
		return true
	case IDE:
		return false
	default:
		return false
	}
}

// UsesSQLiteInject reports whether this target writes state.vscdb.
func UsesSQLiteInject(t Target) bool {
	switch t {
	case IDE:
		return true
	case Classic:
		// Modern classic often uses credential store only; older builds need SQLite.
		// Dual-write classic: credential store + SQLite when available.
		return true
	case Agy:
		return false
	default:
		return false
	}
}

// RestartsProcess is true when switch should close/reopen a GUI process.
func RestartsProcess(t Target) bool {
	return t == IDE || t == Classic
}

// Label is a short human name.
func Label(t Target) string {
	switch t {
	case Classic:
		return "Antigravity (classic)"
	case IDE:
		return "Antigravity IDE"
	case Agy:
		return "Antigravity CLI (agy)"
	case All:
		return "all targets"
	default:
		return string(t)
	}
}

// SettingKey is the settings table key for active account id on this target.
func SettingKey(t Target) string {
	return "active_cloud_account." + string(t)
}
