//go:build !darwin && !windows

package account

import "errors"

// newDecryptor reports that account info is unavailable where cookie decryption is
// not implemented. Fetch/FetchMany then degrade to empty results.
func newDecryptor() (sessionDecryptor, error) {
	return nil, errors.New("account info is unsupported on this platform")
}
