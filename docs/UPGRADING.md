# Upgrading

Changes that may require you to do something, newest first. Storages written by earlier versions stay readable,
and earlier versions can read storages written by this one.

## Unreleased

### SFTP host keys (`sftp://`, `sftpc://`)

- Host keys are now checked by **host name** against Duplicacy's `.duplicacy/known_hosts`, `~/.ssh/known_hosts`
  and `/etc/ssh/ssh_known_hosts`. Keys recorded by earlier versions (by IP address) are still accepted and are
  migrated automatically on the next connection.
- The first connection to a new server shows the key fingerprint and asks for confirmation. **Unattended jobs
  (cron, `-background`, no terminal) refuse unknown keys.** Either run one command interactively first, add the
  server to `~/.ssh/known_hosts` (e.g. with `ssh-keyscan` after checking the fingerprint), or set
  `DUPLICACY_SSH_ACCEPT_NEW_HOST_KEY=1` for the first run.
- A changed host key is refused as before; the error now names the file and line to fix.

### Restore

- An existing **symbolic link** where the snapshot has a file or directory is no longer followed. With
  `-overwrite` it is replaced by the file or directory from the snapshot; without it, the entry fails (the restore
  reports it and exits with an error). If you deliberately replaced a directory with a symbolic link (for example,
  to move data to another disk), restore into the link target directly instead.
- Entries whose paths are absolute or contain `..` are skipped and reported as failures.
- **setuid/setgid bits** are restored only when ownership is restored too (the default). With `-ignore-owner` they
  are dropped.

### Backup

- New option `-strict`: exit with code **4** when any file or directory was skipped because of an access error.
  Without it the exit code is 0 as before.

### Encryption and passwords

- New encrypted storages use **600,000** PBKDF2 iterations (was 16,384). Existing storages keep their setting; to
  upgrade one, run `duplicacy password -iterations 600000`. You can keep the same password. Every computer using
  the storage keeps working, since the iteration count is stored in the config file.
- A warning is printed for passwords shorter than 12 characters and for fewer than 100,000 iterations.
- `duplicacy password` no longer leaves a storage without a config file when saving the new config fails.
- Prompts for cloud secrets (S3 secret keys, B2/Azure/Swift/Storj keys, OAuth client secrets, Dropbox and File
  Fabric tokens, WebDAV and SMB passwords) no longer echo what you type.

### RSA private keys

- Keys in the OpenSSH format (`ssh-keygen` default) and unencrypted PKCS#8 keys are now accepted. Keys with the
  legacy PEM encryption still work but print a warning; convert them with `ssh-keygen -p -f <key file>`.
  Encrypted PKCS#8 keys must be converted the same way.

### File permissions

- The snapshot cache (`.duplicacy/cache`), new `.duplicacy` directories and OAuth token files are now readable only
  by their owner. If another account needs to read the cache, adjust that account's setup rather than the cache
  permissions: the cache contains decrypted file names.

### Platforms

- Supported platforms are Linux x86-64 and macOS. Windows builds are no longer produced or tested by this fork.
