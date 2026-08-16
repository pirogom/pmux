//go:build !windows

package ssh

import "errors"

var errNotSupported = errors.New("ssh config encryption is only supported on Windows")

func encryptData(plain []byte) (string, error) {
	return "", errNotSupported
}

func decryptData(enc string) ([]byte, error) {
	return nil, errNotSupported
}
