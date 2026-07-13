package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shyim/agm/internal/paths"
	"github.com/shyim/agm/internal/target"
)

// SwitchOptions controls process lifecycle around injection.
type SwitchOptions struct {
	Target target.Target
	// Inject performs credential / sqlite writes while processes are stopped (if applicable).
	Inject func() error
	// Restart starts the GUI after inject when target restarts processes.
	// For agy, no restart is performed.
	NoRestart bool
}

// SwitchFlow runs the switch lifecycle for one concrete target (not All — expand first).
func SwitchFlow(opts SwitchOptions) error {
	t := opts.Target
	if t == target.All {
		return fmt.Errorf("expand target.All before SwitchFlow")
	}

	// agy CLI: write credentials only — no GUI kill/start (switchFlow.ts isCliTarget)
	if t == target.Agy {
		if opts.Inject == nil {
			return fmt.Errorf("missing inject")
		}
		return opts.Inject()
	}

	_ = KillForTarget(t)
	time.Sleep(time.Second)

	if opts.Inject != nil {
		if err := opts.Inject(); err != nil {
			return err
		}
	}

	if opts.NoRestart {
		return nil
	}
	if err := StartForTarget(t); err != nil {
		return fmt.Errorf("token injected but failed to start %s: %w", target.Label(t), err)
	}
	return nil
}

// KillForTarget terminates processes for ide or classic.
func KillForTarget(t target.Target) error {
	switch t {
	case target.IDE:
		return killHints([]string{"Antigravity IDE", "AntigravityIDE"})
	case target.Classic:
		// Avoid killing IDE when switching classic only
		if runtime.GOOS == "darwin" {
			_ = exec.Command("pkill", "-f", "Antigravity.app").Run()
			// Do not pkill "Antigravity IDE"
			return nil
		}
		return killHints([]string{"Antigravity.exe", "Antigravity"})
	case target.Agy:
		return nil
	default:
		return KillAntigravity()
	}
}

// KillAntigravity terminates broad Antigravity GUI processes.
func KillAntigravity() error {
	return killHints([]string{"Antigravity", "antigravity"})
}

// StartForTarget launches the right app for the target.
func StartForTarget(t target.Target) error {
	exe := FindExecutable(t)
	if exe == "" {
		return fmt.Errorf("could not locate executable for %s", target.Label(t))
	}
	cmd := exec.Command(exe)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// StartAntigravity launches classic/default desktop if found.
func StartAntigravity() error {
	return StartForTarget(target.Classic)
}

// FindExecutable returns a path for the given target.
func FindExecutable(t target.Target) string {
	switch t {
	case target.Agy:
		return FindAgyExecutable()
	case target.IDE:
		if p := paths.FindExecutableForProduct("ide"); p != "" {
			return p
		}
		return FindRunningExecutable(target.IDE)
	case target.Classic:
		if p := paths.FindExecutableForProduct("classic"); p != "" {
			return p
		}
		return FindRunningExecutable(target.Classic)
	default:
		return paths.FindAntigravityExecutable()
	}
}

// FindAgyExecutable locates the Antigravity CLI binary.
func FindAgyExecutable() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "agy"),
		filepath.Join(home, "bin", "agy"),
		"/usr/local/bin/agy",
		"/usr/bin/agy",
		filepath.Join(home, ".gemini", "antigravity-cli", "bin", "agy"),
	}
	if p, err := exec.LookPath("agy"); err == nil {
		candidates = append([]string{p}, candidates...)
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

func killHints(hints []string) error {
	switch runtime.GOOS {
	case "windows":
		for _, h := range hints {
			name := h
			if !strings.HasSuffix(strings.ToLower(name), ".exe") {
				name += ".exe"
			}
			_ = exec.Command("taskkill", "/F", "/IM", name).Run()
		}
		return nil
	case "darwin":
		for _, h := range hints {
			_ = exec.Command("pkill", "-f", h).Run()
		}
		return nil
	default:
		for _, h := range hints {
			_ = exec.Command("pkill", "-f", h).Run()
		}
		return nil
	}
}

// FindRunningExecutable returns path of a running process matching target.
func FindRunningExecutable(t ...target.Target) string {
	want := target.Classic
	if len(t) > 0 {
		want = t[0]
	}
	switch runtime.GOOS {
	case "darwin", "linux":
		out, err := exec.Command("ps", "-ax", "-o", "command=").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			lower := strings.ToLower(line)
			if !strings.Contains(lower, "antigravity") || strings.Contains(lower, "manager") {
				continue
			}
			if want == target.IDE && !strings.Contains(lower, "ide") {
				// allow "Antigravity IDE"
				if !strings.Contains(line, "Antigravity IDE") {
					continue
				}
			}
			if want == target.Classic && strings.Contains(lower, "ide") {
				continue
			}
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) > 0 && fileExists(fields[0]) {
				return fields[0]
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
