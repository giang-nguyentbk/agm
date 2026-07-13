package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const DBName = "cloud_accounts.db"
const MasterKeyFile = ".mk"

// AgentDir is the local data directory for agm.
// Override: AGM_DATA_DIR or ANTIGRAVITY_AGENT_DIR.
func AgentDir() string {
	if v := os.Getenv("AGM_DATA_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("ANTIGRAVITY_AGENT_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".antigravity-agent")
}

// EnsureAgentDir creates the standalone data directory.
func EnsureAgentDir() (string, error) {
	dir := AgentDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir %s: %w", dir, err)
	}
	return dir, nil
}

// CloudAccountsDBPath is the DB path used by this CLI (always under AgentDir unless AGM_DB_PATH).
func CloudAccountsDBPath() string {
	if db := os.Getenv("AGM_DB_PATH"); db != "" {
		return db
	}
	return filepath.Join(AgentDir(), DBName)
}

// MasterKeyPath is where the standalone CLI stores its AES key (plain hex, mode 0600).
func MasterKeyPath() string {
	return filepath.Join(AgentDir(), MasterKeyFile)
}

// ElectronUserDataDirs returns legacy Antigravity Electron userData candidates (optional key sources).
func ElectronUserDataDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		dirs = append(dirs,
			filepath.Join(appData, "Antigravity Manager"),
			filepath.Join(appData, "AntigravityManager"),
			filepath.Join(appData, "antigravity-manager"),
		)
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		dirs = append(dirs,
			filepath.Join(base, "Antigravity Manager"),
			filepath.Join(base, "AntigravityManager"),
			filepath.Join(base, "antigravity-manager"),
		)
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		dirs = append(dirs,
			filepath.Join(xdg, "Antigravity Manager"),
			filepath.Join(xdg, "AntigravityManager"),
			filepath.Join(xdg, "antigravity-manager"),
		)
	}
	return uniqueStrings(dirs)
}

// ManagerDataDirs returns all dirs that may hold DB / .mk (standalone first).
func ManagerDataDirs() []string {
	dirs := []string{AgentDir()}
	dirs = append(dirs, ElectronUserDataDirs()...)
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, ".config"), cwd)
	}
	return uniqueStrings(dirs)
}

func uniqueStrings(dirs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// FindDBPath returns an existing DB, or empty if none (use EnsureStore to create).
func FindDBPath() string {
	if db := os.Getenv("AGM_DB_PATH"); db != "" && fileExists(db) {
		return db
	}
	canonical := CloudAccountsDBPath()
	if fileExists(canonical) {
		return canonical
	}
	for _, d := range ManagerDataDirs() {
		p := filepath.Join(d, DBName)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// FindMasterKeyPath returns first existing .mk (standalone agent dir preferred).
func FindMasterKeyPath() string {
	p := MasterKeyPath()
	if fileExists(p) {
		return p
	}
	for _, d := range ElectronUserDataDirs() {
		c := filepath.Join(d, MasterKeyFile)
		if fileExists(c) {
			return c
		}
	}
	for _, d := range ManagerDataDirs() {
		c := filepath.Join(d, MasterKeyFile)
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// FindLocalStatePath returns Chromium Local State for legacy Electron OSCrypt keys.
func FindLocalStatePath() string {
	for _, d := range ElectronUserDataDirs() {
		p := filepath.Join(d, "Local State")
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// PrimaryManagerDir is the writable standalone data dir.
func PrimaryManagerDir() string {
	return AgentDir()
}

// productFolderNames returns Application Support / Roaming folder names for a product.
// product is "ide".
func productFolderNames(product string) []string {
	switch product {
	case "ide":
		return []string{"Antigravity IDE", "AntigravityIDE"}
	default:
		return []string{"Antigravity IDE"}
	}
}

// FindStateDB locates state.vscdb for ide (empty product = either/IDE/Code).
func FindStateDB(product string) string {
	home, _ := os.UserHomeDir()
	names := productFolderNames(product)
	if product == "" {
		names = []string{"Antigravity IDE", "Code"}
	}
	var bases []string
	switch runtime.GOOS {
	case "windows":
		bases = []string{os.Getenv("APPDATA")}
	case "darwin":
		bases = []string{filepath.Join(home, "Library", "Application Support")}
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		bases = []string{xdg}
	}
	for _, base := range bases {
		for _, name := range names {
			for _, rel := range []string{
				filepath.Join(name, "User", "globalStorage", "state.vscdb"),
				filepath.Join(name, "User", "state.vscdb"),
				filepath.Join(name, "state.vscdb"),
			} {
				p := filepath.Join(base, rel)
				if fileExists(p) {
					return p
				}
			}
		}
	}
	return ""
}

// FindIDEStateDB locates Antigravity IDE state.vscdb.
func FindIDEStateDB() string {
	return FindStateDB("ide")
}

// FindExecutableForProduct returns a GUI executable for ide.
func FindExecutableForProduct(product string) string {
	home, _ := os.UserHomeDir()
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		prog := os.Getenv("ProgramFiles")
		if product == "ide" {
			candidates = []string{
				filepath.Join(local, "Programs", "Antigravity IDE", "Antigravity IDE.exe"),
				filepath.Join(prog, "Antigravity IDE", "Antigravity IDE.exe"),
			}
		}
	case "darwin":
		if product == "ide" {
			candidates = []string{
				"/Applications/Antigravity IDE.app/Contents/MacOS/Electron",
				"/Applications/Antigravity IDE.app/Contents/MacOS/Antigravity",
				filepath.Join(home, "Applications", "Antigravity IDE.app", "Contents", "MacOS", "Electron"),
			}
		}
	default:
		if product == "ide" {
			candidates = []string{"/usr/bin/antigravity-ide", "/usr/local/bin/antigravity-ide"}
		}
	}
	for _, p := range candidates {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// FindAntigravityExecutable tries common IDE install locations.
func FindAntigravityExecutable() string {
	home, _ := os.UserHomeDir()
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		prog := os.Getenv("ProgramFiles")
		candidates = []string{
			filepath.Join(local, "Programs", "Antigravity IDE", "Antigravity IDE.exe"),
			filepath.Join(prog, "Antigravity IDE", "Antigravity IDE.exe"),
		}
	case "darwin":
		candidates = []string{
			"/Applications/Antigravity IDE.app/Contents/MacOS/Electron",
			"/Applications/Antigravity IDE.app/Contents/MacOS/Antigravity",
			filepath.Join(home, "Applications", "Antigravity IDE.app", "Contents", "MacOS", "Electron"),
		}
	default:
		candidates = []string{
			"/usr/bin/antigravity-ide",
			"/usr/local/bin/antigravity-ide",
			filepath.Join(home, ".local", "bin", "antigravity-ide"),
		}
	}

	for _, p := range candidates {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// MatchEmail reports whether haystack contains needle (case-insensitive).
func MatchEmail(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
