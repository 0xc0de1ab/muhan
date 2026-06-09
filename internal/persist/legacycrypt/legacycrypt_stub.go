//go:build !cgo || !linux

package legacycrypt

import "errors"

var ErrPasswordContainsNUL = errors.New("legacy crypt password contains NUL")

func Hash(password string) (string, error) {
	return "", errors.New("legacy crypt requires cgo on linux")
}
