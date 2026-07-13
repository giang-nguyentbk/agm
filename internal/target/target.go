package target

import (
	"fmt"
	"strings"
)

// Target is an Antigravity product surface.
type Target string

const (
	IDE Target = "ide" // Antigravity IDE
	Agy Target = "agy" // Antigravity CLI (agy)
	All Target = "all" // apply to every relevant surface
)

// Parse normalizes a user-supplied target name.
func Parse(s string) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all", "both", "*":
		return All, nil
	case "ide":
		return IDE, nil
	case "agy", "cli", "antigravity-cli", "antigravity_cli":
		return Agy, nil
	default:
		return "", fmt.Errorf("unknown target %q (want ide|agy|all)", s)
	}
}

// Expand turns All into concrete targets for injection order.
func Expand(t Target) []Target {
	if t == All {
		// CLI first (no process restart), then IDE
		return []Target{Agy, IDE}
	}
	return []Target{t}
}

// UsesCredentialStore reports whether this target writes the OS credential store
// (see CredentialStoreInjectionAdapter.shouldInjectTokenIntoCredentialStore).
func UsesCredentialStore(t Target) bool {
	return t == Agy
}

// UsesSQLiteInject reports whether this target writes state.vscdb.
func UsesSQLiteInject(t Target) bool {
	return t == IDE
}

// RestartsProcess is true when switch should close/reopen a GUI process.
func RestartsProcess(t Target) bool {
	return t == IDE
}

// Label is a short human name.
func Label(t Target) string {
	switch t {
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
