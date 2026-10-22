// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bufio"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// SFTP host keys are verified against the standard known_hosts files: Duplicacy's own file in the preference
// directory (the only one written to), the user's ~/.ssh/known_hosts and the system-wide ssh_known_hosts.
// Entries are matched by host name (and port), so a changed DNS record can't silently introduce a new key.
//
// Duplicacy used to record keys as '<ip>:<port> <key>' lines; those are still honored and migrated.

// acceptNewHostKeyVariable can be set to accept and record the key of a host seen for the first time without a
// prompt, e.g. for unattended setups.  A changed key is never accepted.
const acceptNewHostKeyVariable = "DUPLICACY_SSH_ACCEPT_NEW_HOST_KEY"

// sftpHostKeyPrompt asks the user to confirm the key of a new host.  It is a variable so that tests can replace it.
var sftpHostKeyPrompt = promptForNewHostKey

var knownHostsLock sync.Mutex

// sftpKnownHostsFiles returns Duplicacy's known_hosts file followed by the other existing known_hosts files.
func sftpKnownHostsFiles() (duplicacyFile string, others []string, err error) {
	if preferencePath == "" {
		return "", nil, fmt.Errorf("Can't verify SSH host since the preference path is not set")
	}
	duplicacyFile = path.Join(preferencePath, "known_hosts")

	candidates := []string{"/etc/ssh/ssh_known_hosts"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append([]string{filepath.Join(home, ".ssh", "known_hosts")}, candidates...)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			others = append(others, candidate)
		}
	}
	return duplicacyFile, others, nil
}

// loadLegacyHostKeys returns the keys recorded by earlier versions, keyed by '<ip>:<port>'.
func loadLegacyHostKeys(file string) map[string]string {
	keys := make(map[string]string)
	content, err := os.ReadFile(file)
	if err != nil {
		return keys
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "[") ||
			strings.HasPrefix(fields[0], "|") {
			continue
		}
		if host, port, err := net.SplitHostPort(fields[0]); err == nil && net.ParseIP(host) != nil && port != "" {
			keys[fields[0]] = fields[1] + " " + fields[2]
		}
	}
	return keys
}

func appendKnownHost(file string, hostname string, key ssh.PublicKey) error {
	knownHostsLock.Lock()
	defer knownHostsLock.Unlock()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n")
	return err
}

func authorizedKeyString(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

// newSFTPHostKeyChecker returns the host key callback for 'serverAddress' ('host:port'), and the host key
// algorithms to prefer, so that a server offering several key types presents one that is already known.
func newSFTPHostKeyChecker(serverAddress string) (ssh.HostKeyCallback, []string, error) {
	duplicacyFile, others, err := sftpKnownHostsFiles()
	if err != nil {
		return nil, nil, err
	}
	if f, err := os.OpenFile(duplicacyFile, os.O_RDONLY|os.O_CREATE, 0600); err != nil {
		return nil, nil, err
	} else {
		f.Close()
	}

	files := append([]string{duplicacyFile}, others...)
	known, err := knownhosts.New(files...)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to read the known hosts files %v: %v", files, err)
	}
	legacy := loadLegacyHostKeys(duplicacyFile)

	callback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := known(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyError *knownhosts.KeyError
		if !errors.As(err, &keyError) {
			// For instance, the key has been revoked
			return err
		}

		if len(keyError.Want) > 0 {
			for _, want := range keyError.Want {
				LOG_WARN("HOSTKEY_OLD", "The known key for '%s' is %s %s (%s, line %d)", hostname, want.Key.Type(),
					ssh.FingerprintSHA256(want.Key), want.Filename, want.Line)
			}
			LOG_WARN("HOSTKEY_NEW", "The host presented %s %s", key.Type(), ssh.FingerprintSHA256(key))
			return fmt.Errorf("The host key for '%s' has changed; this could mean someone is intercepting the "+
				"connection.  If the change is expected, remove the old key from the file and line listed above", hostname)
		}

		// Not in any known_hosts file; check what earlier versions recorded for this address
		if recorded, found := legacy[remote.String()]; found {
			if recorded != authorizedKeyString(key) {
				LOG_WARN("HOSTKEY_OLD", "The existing key for '%s' is %s (file %s)", remote.String(), recorded, duplicacyFile)
				LOG_WARN("HOSTKEY_NEW", "The new key is '%s'", authorizedKeyString(key))
				return fmt.Errorf("The host key for '%s' has changed", remote.String())
			}
			LOG_INFO("HOSTKEY_MIGRATE", "Recording the known key of %s for '%s'", remote.String(), hostname)
			return appendKnownHost(duplicacyFile, hostname, key)
		}

		if !sftpHostKeyPrompt(hostname, key) {
			return fmt.Errorf("The host key for '%s' (%s %s) is not known.  Connect once interactively to confirm "+
				"it, add it to %s, or set %s=1 to accept new host keys", hostname, key.Type(),
				ssh.FingerprintSHA256(key), duplicacyFile, acceptNewHostKeyVariable)
		}
		return appendKnownHost(duplicacyFile, hostname, key)
	}

	return callback, preferredHostKeyAlgorithms(known, serverAddress), nil
}

// preferredHostKeyAlgorithms lists the algorithms of the keys already known for the server first, followed by all
// supported ones.  It returns nil (the library default) if no key is known.
func preferredHostKeyAlgorithms(known ssh.HostKeyCallback, serverAddress string) []string {
	// Probing with a key that can't match reveals the known keys for the host
	probe, err := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	if err != nil {
		return nil
	}
	var keyError *knownhosts.KeyError
	if !errors.As(known(serverAddress, &net.TCPAddr{IP: net.IPv4zero}, probe), &keyError) || len(keyError.Want) == 0 {
		return nil
	}

	var algorithms []string
	seen := make(map[string]bool)
	add := func(algorithm string) {
		if !seen[algorithm] {
			seen[algorithm] = true
			algorithms = append(algorithms, algorithm)
		}
	}
	for _, want := range keyError.Want {
		if want.Key.Type() == ssh.KeyAlgoRSA {
			add(ssh.KeyAlgoRSASHA512)
			add(ssh.KeyAlgoRSASHA256)
		}
		add(want.Key.Type())
	}
	for _, algorithm := range ssh.SupportedAlgorithms().HostKeys {
		add(algorithm)
	}
	return algorithms
}

func promptForNewHostKey(hostname string, key ssh.PublicKey) bool {
	fingerprint := ssh.FingerprintSHA256(key)
	switch strings.ToLower(os.Getenv(acceptNewHostKeyVariable)) {
	case "1", "yes", "true":
		LOG_WARN("HOSTKEY_ACCEPT", "Accepting the new %s key %s for '%s' because %s is set", key.Type(), fingerprint,
			hostname, acceptNewHostKeyVariable)
		return true
	}

	if RunInBackground || !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}

	fmt.Printf("The authenticity of host '%s' can't be established.\n", hostname)
	fmt.Printf("%s key fingerprint is %s.\n", key.Type(), fingerprint)
	fmt.Printf("Are you sure you want to continue connecting (yes/no)? ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}
