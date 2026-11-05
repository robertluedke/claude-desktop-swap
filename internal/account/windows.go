//go:build windows

package account

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

const (
	localStateFile = "Local State"
	dpapiPrefix    = "DPAPI"
	v10Prefix      = "v10"
	gcmNonceLen    = 12
)

// dpapiDecryptor decrypts Chromium v10 cookies: AES-256-GCM under a key that
// Local State stores wrapped by DPAPI for the current Windows user. Each
// profile snapshot carries its own Local State, so keys are resolved per tree.
type dpapiDecryptor struct {
	mu   sync.Mutex
	keys map[string][]byte
}

func newDecryptor() (sessionDecryptor, error) {
	return &dpapiDecryptor{keys: map[string][]byte{}}, nil
}

func (d *dpapiDecryptor) SessionKey(cookiesPath string) (string, error) {
	key, err := d.key(appDataRoot(cookiesPath))
	if err != nil {
		return "", err
	}
	blob, err := readSessionBlob(cookiesPath)
	if err != nil {
		return "", err
	}
	return decryptV10(key, blob)
}

// appDataRoot maps <root>/Network/Cookies (or legacy <root>/Cookies) to <root>.
func appDataRoot(cookiesPath string) string {
	dir := filepath.Dir(cookiesPath)
	if strings.EqualFold(filepath.Base(dir), "Network") {
		return filepath.Dir(dir)
	}
	return dir
}

func (d *dpapiDecryptor) key(root string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if key, ok := d.keys[root]; ok {
		return key, nil
	}
	data, err := os.ReadFile(filepath.Join(root, localStateFile))
	if err != nil {
		return nil, err
	}
	var state struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	wrapped, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, err
	}
	wrapped, ok := bytes.CutPrefix(wrapped, []byte(dpapiPrefix))
	if !ok {
		return nil, errors.New("Local State key is not DPAPI-protected")
	}
	key, err := unprotect(wrapped)
	if err != nil {
		return nil, fmt.Errorf("dpapi: %w", err)
	}
	d.keys[root] = key
	return key, nil
}

func unprotect(data []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), nil
}

func readSessionBlob(cookiesPath string) ([]byte, error) {
	dsn := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(cookiesPath), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var blob []byte
	err = db.QueryRow(`SELECT encrypted_value FROM cookies WHERE host_key IN ('.claude.ai', 'claude.ai') AND name = 'sessionKey' LIMIT 1`).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sessionKey cookie not found in %s", cookiesPath)
	}
	return blob, err
}

func decryptV10(key, blob []byte) (string, error) {
	sealed, ok := bytes.CutPrefix(blob, []byte(v10Prefix))
	if !ok {
		return "", errors.New("unsupported cookie encryption version")
	}
	if len(sealed) < gcmNonceLen {
		return "", errors.New("cookie ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, sealed[:gcmNonceLen], sealed[gcmNonceLen:], nil)
	if err != nil {
		return "", err
	}
	return string(stripDomainPrefix(plain)), nil
}
