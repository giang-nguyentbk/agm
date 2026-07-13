//go:build !darwin

package crypto

import "errors"

func loadKeychainMasterKey() (MasterKey, error) {
	return nil, errors.New("keychain master key not implemented on this platform")
}
