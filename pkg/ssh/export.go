package ssh

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

// exportFile is the portable (password-encrypted) file format used by
// Export/Import so data can be moved between machines.
type exportFile struct {
	Version int    `json:"version"`
	Salt    []byte `json:"salt"`
	Data    []byte `json:"data"` // nonce || AES-256-GCM ciphertext of the config JSON
}

// Export encrypts cfg with the user-provided password and returns the bytes
// to be written to an export file.
func Export(cfg *Config, password string) ([]byte, error) {
	if cfg == nil {
		cfg = Default()
	}
	if password == "" {
		return nil, errors.New("export password cannot be empty")
	}

	plain, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	key, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	out := exportFile{
		Version: ConfigVersion,
		Salt:    salt,
		Data:    gcm.Seal(nonce, nonce, plain, nil),
	}
	return json.Marshal(out)
}

// Import decrypts an export file with the user-provided password. It returns
// an error if the password is wrong or the file is corrupted (AES-GCM
// authentication failure), or if the stored version is incompatible.
func Import(data []byte, password string) (*Config, error) {
	var in exportFile
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, errors.New("invalid export file")
	}

	if in.Version != ConfigVersion {
		return nil, ErrUnsupportedVersion
	}

	key, err := scrypt.Key([]byte(password), in.Salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(in.Data) < nonceSize {
		return nil, errors.New("invalid export file")
	}

	nonce, sealed := in.Data[:nonceSize], in.Data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, errors.New("incorrect password or corrupted export file")
	}

	var cfg Config
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}

	if cfg.ClientPath == "" {
		cfg.ClientPath = DefaultClientPath
	}
	if cfg.Addresses == nil {
		cfg.Addresses = []Address{}
	}
	return &cfg, nil
}
