//go:build !windows

package crypto

import "errors"

func dpapiUnprotect(_ []byte) ([]byte, error) {
	return nil, errors.New("DPAPI is only available on Windows")
}
