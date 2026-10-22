# Security model

What Duplicacy protects, against whom, and the known limits. See [FORMAT.md](FORMAT.md) for how the data is
encoded.

## Assets

- **Backup content**: file data, file names, sizes, timestamps, ownership and extended attributes.
- **Storage keys**: the keys in the `config` file, protected by the storage password (and optionally an RSA key).
- **Credentials**: storage passwords, cloud keys and tokens, SSH keys and passphrases.
- **The machine being restored to**: a restore must not be able to change anything outside its target directory.

## Who can do what

### Someone with read access to the storage (cloud provider, stolen disk)

| Encrypted storage | Unencrypted storage |
|-------------------|---------------------|
| Can't read content or file names (AES-256-GCM with per-chunk keys). | Can read everything. |
| Can see the number and sizes of chunks (padded to 256 bytes), snapshot ids, revision numbers and times. | — |
| Can try to guess the password offline against the `config` file; the cost is set by the PBKDF2 iteration count (600,000 for new storages). | — |
| Can, in principle, learn something about content from chunk sizes (content-defined chunking attacks, see upstream #676). Practical attacks need chosen or known data. | Can tell whether a known file is stored, since chunk ids are computed with public keys. |

With RSA encryption, file chunks additionally need the RSA private key, so a machine that only backs up (and
holds only the public key) can't read old backups even with the storage password. Metadata chunks (file lists)
are not RSA-encrypted, so the storage password alone reveals file names.

### Someone with write access to the storage

They can always destroy or withhold backups; Duplicacy can't prevent that (use storage-side immutability or
versioning for ransomware protection). What Duplicacy does guarantee:

- **Encrypted storages**: forged or modified chunks and snapshot files fail authentication (AES-GCM, chunk ids are
  verified after decryption) and are reported as corrupted. Restored data is also checked against file hashes.
- **Restore can't escape its target directory**, even for unencrypted storages where snapshots can be forged:
  entry paths are validated (no absolute paths, `..` or empty components), no symbolic link is followed between
  the target directory and an entry, and an existing symbolic link where the snapshot has a file or directory is
  replaced only with `-overwrite` and never written through. setuid/setgid bits are only restored together with
  ownership.
- **Malformed data can't crash or exhaust the client**: erasure-coding headers, compressed sizes (limited to twice
  the maximum chunk size, at least 256 MiB) and key-derivation iteration counts (at most 100 million) are
  validated before use.

They *can* make a client upload data again or fail operations, and on unencrypted storages they can make a
restore produce wrong (but contained) content.

### A network attacker

- Cloud backends use TLS (`minio://`, `webdav-http://` and Swift with `protocol=http` explicitly opt out).
- SFTP host keys are checked against Duplicacy's `known_hosts`, `~/.ssh/known_hosts` and
  `/etc/ssh/ssh_known_hosts` by host name. An unknown key requires confirmation (or
  `DUPLICACY_SSH_ACCEPT_NEW_HOST_KEY=1` for unattended first connections); a changed key is refused.
- `sftpc://` enables legacy SSH algorithms and prints a warning.

### Other local users

- `.duplicacy`, the snapshot cache (which holds *decrypted* metadata), `preferences`, `known_hosts`, logs and OAuth
  token files are readable only by their owner.
- Passwords are taken from (in order) environment variables, the `preferences` file, the system keyring, or a
  prompt. Storing them in `preferences` or environment variables is the user's choice; prefer the keyring.
- Pre/post scripts in `.duplicacy/scripts` run with the user's privileges and receive the command-line arguments.

## Known limitations

- **Prune** relies on the clocks of the backup clients and on fossil collections stored locally on the machine that
  runs prune. Don't run prune from several machines at the same time; see `REVIEW_PLAN.md` (B6) for planned
  improvements.
- **Resuming an interrupted backup** trusts its local state about which chunks were already uploaded (B4).
- **`check -chunks`** does not re-verify chunks it has verified before (B5).
- The old config format (static salt, 16,384 iterations) is still accepted for reading. Run
  `duplicacy password -iterations 600000` to move such storages to the current format.
