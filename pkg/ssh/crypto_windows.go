//go:build windows

package ssh

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dataBlob mirrors the Win32 DATA_BLOB structure.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32                     = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData        = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData      = crypt32.NewProc("CryptUnprotectData")
	procLocalFree               = windows.NewLazySystemDLL("kernel32.dll").NewProc("LocalFree")
)

const cryptprotectUIForbidden = 0x1

// encryptData protects plain using Windows DPAPI, binding it to the current
// Windows user account on the current machine. The returned string is base64
// of the encrypted blob.
func encryptData(plain []byte) (string, error) {
	if len(plain) == 0 {
		return "", nil
	}

	in := dataBlob{
		cbData: uint32(len(plain)),
		pbData: &plain[0],
	}
	var out dataBlob

	r1, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // szDataDescr
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		cryptprotectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("CryptProtectData failed: %v", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))

	blob := make([]byte, out.cbData)
	copy(blob, unsafe.Slice(out.pbData, out.cbData))
	return base64.StdEncoding.EncodeToString(blob), nil
}

// decryptData reverses encryptData. It fails with an error when called for a
// different Windows user account or on a different machine.
func decryptData(enc string) ([]byte, error) {
	if enc == "" {
		return []byte{}, nil
	}

	blob, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("invalid encrypted payload: %w", err)
	}

	in := dataBlob{
		cbData: uint32(len(blob)),
		pbData: &blob[0],
	}
	var out dataBlob

	r1, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // pDataDescr
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		cryptprotectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed (data is bound to the original Windows user/machine): %v", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))

	dec := make([]byte, out.cbData)
	copy(dec, unsafe.Slice(out.pbData, out.cbData))
	return dec, nil
}
