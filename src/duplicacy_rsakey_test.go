// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// expectLogError runs 'f' and returns the message of the LOG_ERROR it raises, or "" if it raises none.
func expectLogError(f func()) (message string) {
	savedTestingT := testingT
	testingT = nil
	defer func() {
		testingT = savedTestingT
		if r := recover(); r != nil {
			if e, ok := r.(Exception); ok {
				message = e.Message
				return
			}
			panic(r)
		}
	}()
	f()
	return ""
}

// S7: private keys in the OpenSSH format and unencrypted PKCS#8 are accepted, legacy PEM encryption still works,
// and failures are reported clearly without printing a key passed inline.
func TestLoadRSAPrivateKeyFormats(t *testing.T) {
	SetLoggingLevel(ERROR)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := CreateConfig()
	config.rsaPublicKey = &key.PublicKey

	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	//lint:ignore SA1019 creating a legacy encrypted key to test that it can still be read
	legacy, err := x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), []byte("secret"), x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	openSSH, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	openSSHEncrypted, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	forms := []struct {
		name       string
		block      *pem.Block
		passphrase string
	}{
		{"PKCS#1", &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}, ""},
		{"PKCS#8", &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}, ""},
		{"legacy encrypted PEM", legacy, "secret"},
		{"OpenSSH", openSSH, ""},
		{"encrypted OpenSSH", openSSHEncrypted, "secret"},
	}
	for _, form := range forms {
		config.rsaPrivateKey = nil
		encoded := string(pem.EncodeToMemory(form.block))
		if message := expectLogError(func() { config.loadRSAPrivateKey(encoded, form.passphrase) }); message != "" {
			t.Errorf("%s: %s", form.name, message)
		} else if config.rsaPrivateKey == nil {
			t.Errorf("%s: the key was not loaded", form.name)
		}
	}

	failures := []struct {
		name       string
		block      *pem.Block
		passphrase string
		expected   string
	}{
		{"wrong OpenSSH passphrase", openSSHEncrypted, "wrong", "incorrect passphrase"},
		{"missing OpenSSH passphrase", openSSHEncrypted, "", "requires a passphrase"},
		{"missing legacy passphrase", legacy, "", "requires a passphrase"},
		{"wrong legacy passphrase", legacy, "wrong", "decrypt"},
		{"encrypted PKCS#8", &pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1, 2, 3}}, "secret", "ssh-keygen"},
	}
	for _, failure := range failures {
		config.rsaPrivateKey = nil
		encoded := string(pem.EncodeToMemory(failure.block))
		message := expectLogError(func() { config.loadRSAPrivateKey(encoded, failure.passphrase) })
		if !strings.Contains(message, failure.expected) {
			t.Errorf("%s: expected an error containing %q, got %q", failure.name, failure.expected, message)
		}
		if strings.Contains(message, "BEGIN") || strings.Contains(message, "PRIVATE KEY-----") {
			t.Errorf("%s: the error message contains the key: %q", failure.name, message)
		}
	}
}

// A config whose public key is not an RSA key used to panic on an unchecked type assertion.
func TestConfigRejectsNonRSAPublicKey(t *testing.T) {
	_, edKey, err := ed25519KeyForTest()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _ := x509.MarshalPKIXPublicKey(edKey)
	description := `{"compression-level":100,"average-chunk-size":4194304,"max-chunk-size":16777216,` +
		`"min-chunk-size":1048576,"chunk-seed":"","hash-key":"","id-key":"","chunk-key":"","file-key":"",` +
		`"rsa-public-key":"` + hexForTest(publicKey) + `"}`
	config := CreateConfig()
	if err := config.UnmarshalJSON([]byte(description)); err == nil {
		t.Error("A config with a non-RSA public key was accepted")
	}
}

func ed25519KeyForTest() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	return private, public, err
}

func hexForTest(data []byte) string { return hex.EncodeToString(data) }
