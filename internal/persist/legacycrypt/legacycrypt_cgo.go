//go:build cgo && linux

package legacycrypt

/*
#cgo LDFLAGS: -lcrypt
#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <crypt.h>

static char* muhan_crypt(const char* key, const char* salt) {
	char *out = crypt(key, salt);
	if (out == NULL) {
		return NULL;
	}
	return strdup(out);
}
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
)

const legacySalt = "St"

var ErrPasswordContainsNUL = errors.New("legacy crypt password contains NUL")

func Hash(password string) (string, error) {
	if strings.ContainsRune(password, '\x00') {
		return "", ErrPasswordContainsNUL
	}
	passwordBytes, err := legacykr.EncodeEUCKR(password)
	if err != nil {
		return "", err
	}

	cPassword := C.CString(string(passwordBytes))
	defer C.free(unsafe.Pointer(cPassword))
	cSalt := C.CString(legacySalt)
	defer C.free(unsafe.Pointer(cSalt))

	out := C.muhan_crypt(cPassword, cSalt)
	if out == nil {
		return "", errors.New("legacy crypt failed")
	}
	defer C.free(unsafe.Pointer(out))

	hash := C.GoString(out)
	if len(hash) <= len(legacySalt) {
		return "", errors.New("legacy crypt returned short hash")
	}
	return hash[len(legacySalt):], nil
}
