//go:build darwin

package crypto

import (
	"os/exec"
	"strings"
)

func loadKeychainMasterKey() (MasterKey, error) {
	// keytar service/account used by src/shared/security/security.ts
	out, err := exec.Command(
		"security", "find-generic-password",
		"-s", "AntigravityManager",
		"-a", "MasterKey",
		"-w",
	).Output()
	if err != nil {
		return nil, err
	}
	return parseHexKey(strings.TrimSpace(string(out)))
}
