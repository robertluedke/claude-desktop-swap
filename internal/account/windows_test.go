//go:build windows

package account

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protect(t *testing.T, data []byte) []byte {
	t.Helper()
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		t.Fatal(err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size))
}

func TestDPAPIDecryptorReadsSyntheticCookie(t *testing.T) {
	root := t.TempDir()
	key := make([]byte, 32)
	nonce := make([]byte, gcmNonceLen)
	_, _ = rand.Read(key)
	_, _ = rand.Read(nonce)

	state, _ := json.Marshal(map[string]any{"os_crypt": map[string]string{
		"encrypted_key": base64.StdEncoding.EncodeToString(append([]byte(dpapiPrefix), protect(t, key)...)),
	}})
	if err := os.WriteFile(filepath.Join(root, localStateFile), state, 0600); err != nil {
		t.Fatal(err)
	}

	token := "sk-ant-sid01-synthetic"
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain := append(bytes.Repeat([]byte{0xfe}, domainHashPrefixLen), token...)
	blob := append([]byte(v10Prefix), gcm.Seal(nonce, nonce, plain, nil)...)

	cookies := filepath.Join(root, "Network", "Cookies")
	if err := os.MkdirAll(filepath.Dir(cookies), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cookies)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cookies (host_key TEXT, name TEXT, encrypted_value BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('.claude.ai', 'sessionKey', ?)`, blob); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	d, _ := newDecryptor()
	got, err := d.SessionKey(cookies)
	if err != nil || got != token {
		t.Fatalf("SessionKey = %q, %v", got, err)
	}
}

func TestDecryptV10RejectsOtherVersions(t *testing.T) {
	if _, err := decryptV10(make([]byte, 32), []byte("v20abcdefghijklmnop")); err == nil {
		t.Fatal("expected v20 to be rejected")
	}
}
