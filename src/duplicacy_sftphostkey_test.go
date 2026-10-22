// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func newTestHostKey(t *testing.T) ssh.PublicKey {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// setupHostKeyTest isolates the known_hosts files: a temporary preference directory and home directory.
func setupHostKeyTest(t *testing.T, accept bool) (knownHostsFile string, home string) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	preferences := t.TempDir()
	SetDuplicacyPreferencePath(preferences)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(acceptNewHostKeyVariable, "")

	saved := sftpHostKeyPrompt
	sftpHostKeyPrompt = func(hostname string, key ssh.PublicKey) bool { return accept }
	t.Cleanup(func() { sftpHostKeyPrompt = saved })
	return filepath.Join(preferences, "known_hosts"), home
}

func checkHost(t *testing.T, address string, ip string, key ssh.PublicKey) error {
	checker, _, err := newSFTPHostKeyChecker(address)
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(address)
	portNumber, _ := strconv.Atoi(port)
	return checker(address, &net.TCPAddr{IP: net.ParseIP(ip), Port: portNumber}, key)
}

func TestSFTPHostKeyNewHostRequiresConfirmation(t *testing.T) {
	file, _ := setupHostKeyTest(t, false)
	key := newTestHostKey(t)

	if err := checkHost(t, "backup.example.com:22", "192.0.2.1", key); err == nil {
		t.Fatal("An unknown host key was accepted without confirmation")
	}
	if content, _ := os.ReadFile(file); len(content) != 0 {
		t.Errorf("A rejected key was recorded: %s", content)
	}

	// The environment variable accepts new keys without a prompt (the real prompt is bypassed here)
	sftpHostKeyPrompt = promptForNewHostKey
	t.Setenv(acceptNewHostKeyVariable, "1")
	if err := checkHost(t, "backup.example.com:22", "192.0.2.1", key); err != nil {
		t.Fatalf("A new key was not accepted with %s=1: %v", acceptNewHostKeyVariable, err)
	}
}

func TestSFTPHostKeyMatchesByHostName(t *testing.T) {
	file, _ := setupHostKeyTest(t, true)
	key := newTestHostKey(t)

	if err := checkHost(t, "backup.example.com:2222", "192.0.2.1", key); err != nil {
		t.Fatalf("A confirmed key was rejected: %v", err)
	}
	content, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(content), "[backup.example.com]:2222 ssh-ed25519 ") {
		t.Errorf("The key was not recorded in OpenSSH format: %q", content)
	}

	// Once known, the host is never prompted for again, whatever address it resolves to
	sftpHostKeyPrompt = func(string, ssh.PublicKey) bool { t.Error("Prompted for a known host"); return false }
	if err := checkHost(t, "backup.example.com:2222", "198.51.100.7", key); err != nil {
		t.Errorf("The known key was rejected at a different address: %v", err)
	}

	// A different key for the same host is an error, even if it would otherwise be confirmed
	if err := checkHost(t, "backup.example.com:2222", "192.0.2.1", newTestHostKey(t)); err == nil {
		t.Error("A changed host key was accepted")
	}
}

func TestSFTPHostKeyLegacyEntries(t *testing.T) {
	file, _ := setupHostKeyTest(t, false)
	key := newTestHostKey(t)
	legacyLine := "192.0.2.1:22 " + authorizedKeyString(key) + "\n"
	if err := os.WriteFile(file, []byte(legacyLine), 0600); err != nil {
		t.Fatal(err)
	}

	if err := checkHost(t, "backup.example.com:22", "192.0.2.1", newTestHostKey(t)); err == nil {
		t.Error("A key different from the legacy entry was accepted")
	}
	if err := checkHost(t, "backup.example.com:22", "192.0.2.1", key); err != nil {
		t.Fatalf("The key recorded by an earlier version was rejected: %v", err)
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "\nbackup.example.com ssh-ed25519 ") {
		t.Errorf("The legacy key was not migrated to a host name entry: %q", content)
	}
}

func TestSFTPHostKeyUsesUserKnownHosts(t *testing.T) {
	_, home := setupHostKeyTest(t, false)
	key := newTestHostKey(t)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	line := "backup.example.com " + authorizedKeyString(key) + "\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkHost(t, "backup.example.com:22", "192.0.2.1", key); err != nil {
		t.Errorf("A key from ~/.ssh/known_hosts was rejected: %v", err)
	}
}

func TestSFTPHostKeyAlgorithmPreference(t *testing.T) {
	file, _ := setupHostKeyTest(t, false)
	_, algorithms, err := newSFTPHostKeyChecker("backup.example.com:22")
	if err != nil {
		t.Fatal(err)
	}
	if algorithms != nil {
		t.Errorf("Algorithms %v preferred for an unknown host", algorithms)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _ := ssh.NewPublicKey(&rsaKey.PublicKey)
	os.WriteFile(file, []byte("backup.example.com "+authorizedKeyString(publicKey)+"\n"), 0600)
	_, algorithms, err = newSFTPHostKeyChecker("backup.example.com:22")
	if err != nil {
		t.Fatal(err)
	}
	if len(algorithms) < 3 || algorithms[0] != ssh.KeyAlgoRSASHA512 || algorithms[1] != ssh.KeyAlgoRSASHA256 {
		t.Errorf("RSA algorithms are not preferred for a host with a known RSA key: %v", algorithms)
	}
}

// startTestSFTPServer runs an SSH server with the SFTP subsystem on localhost, using 'hostKey' as its key.
func startTestSFTPServer(t *testing.T, hostKey ed25519.PrivateKey) int {
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == "user" && string(password) == "password" {
				return nil, nil
			}
			return nil, fmt.Errorf("access denied")
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					conn.Close()
					return
				}
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unsupported")
						continue
					}
					channel, channelRequests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go func() {
						for request := range channelRequests {
							isSFTP := request.Type == "subsystem" && len(request.Payload) > 4 && string(request.Payload[4:]) == "sftp"
							request.Reply(isSFTP, nil)
							if isSFTP {
								if server, err := sftp.NewServer(channel); err == nil {
									server.Serve()
								}
								channel.Close()
							}
						}
					}()
				}
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func connectTestSFTP(t *testing.T, port int, storageDir string) error {
	checker, algorithms, err := newSFTPHostKeyChecker(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	storage, err := CreateSFTPStorage(false, "127.0.0.1", port, "user", storageDir, 2,
		[]ssh.AuthMethod{ssh.Password("password")}, checker, algorithms, 1)
	if storage != nil {
		CloseSFTPStorage(storage)
	}
	return err
}

func TestSFTPHostKeyEndToEnd(t *testing.T) {
	setupHostKeyTest(t, false)
	_, hostKey, _ := ed25519.GenerateKey(rand.Reader)
	port := startTestSFTPServer(t, hostKey)
	storageDir := t.TempDir()

	if err := connectTestSFTP(t, port, storageDir); err == nil {
		t.Fatal("Connected to a server with an unconfirmed host key")
	}

	sftpHostKeyPrompt = func(string, ssh.PublicKey) bool { return true }
	if err := connectTestSFTP(t, port, storageDir); err != nil {
		t.Fatalf("Failed to connect after confirming the host key: %v", err)
	}

	sftpHostKeyPrompt = func(string, ssh.PublicKey) bool { t.Error("Prompted for a known host"); return false }
	if err := connectTestSFTP(t, port, storageDir); err != nil {
		t.Fatalf("Failed to reconnect to a known host: %v", err)
	}

	// The same address now presents a different key
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	otherPort := startTestSFTPServer(t, otherKey)
	known, _ := os.ReadFile(filepath.Join(preferencePath, "known_hosts"))
	samePort := strings.Replace(string(known), strconv.Itoa(port), strconv.Itoa(otherPort), 1)
	os.WriteFile(filepath.Join(preferencePath, "known_hosts"), []byte(samePort), 0600)
	if err := connectTestSFTP(t, otherPort, storageDir); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("Connecting to a host with a changed key didn't fail as expected: %v", err)
	}
}
