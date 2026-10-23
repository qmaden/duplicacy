# Command reference

<!-- Generated from duplicacy/duplicacy_main.go by TestCommandReference; do not edit by hand. -->

Usage: `duplicacy [global options] <command> [options] [arguments]`

Run `duplicacy <command> -h` for the same information in the terminal. Passwords and keys can also be given through environment variables; see [storage-backends.md](storage-backends.md).

## Global options

| Option | Description |
|--------|-------------|
| `-verbose` | show more detailed information |
| `-debug` | show even more detailed information, useful for debugging |
| `-log` | enable log-style output |
| `-stack` | print the stack trace when an error occurs |
| `-no-script` | do not run script before or after command execution |
| `-background` | read passwords, tokens, or keys only from keychain/keyring or env |
| `-memory-limit <MB>` | soft limit on the memory used by Duplicacy, in MB (the GOMEMLIMIT environment variable also works) |
| `-profile <address:port>` | enable the profiling tool and listen on the specified address:port |
| `-comment` | add a comment to identify the process |
| `-suppress <id>` | suppress logs with the specified id (can be repeated) |
| `-print-memory-usage` | print memory usage every second |

## Commands

- [`init`](#init): Initialize the storage if necessary and the current directory as the repository
- [`backup`](#backup): Save a snapshot of the repository to the storage
- [`restore`](#restore): Restore the repository to a previously saved snapshot
- [`list`](#list): List snapshots
- [`check`](#check): Check the integrity of snapshots
- [`cat`](#cat): Print to stdout the specified file, or the snapshot content if no file is specified
- [`diff`](#diff): Compare two snapshots or two revisions of a file
- [`history`](#history): Show the history of a file
- [`prune`](#prune): Prune snapshots by revision, tag, or retention policy
- [`password`](#password): Change the storage password
- [`add`](#add): Add an additional storage to be used for the existing repository
- [`set`](#set): Change the options for the default or specified storage
- [`copy`](#copy): Copy snapshots between compatible storages
- [`info`](#info): Show the information about the specified storage
- [`benchmark`](#benchmark): Run a set of benchmarks to test download and upload speeds

### init

Initialize the storage if necessary and the current directory as the repository

```
duplicacy init [options] <snapshot id> <storage url>
```

| Option | Description |
|--------|-------------|
| `-encrypt` | encrypt the storage with a password |
| `-chunk-size <size>` | the average size of chunks (defaults to 4M) |
| `-max-chunk-size <size>` | the maximum size of chunks (defaults to chunk-size*4) |
| `-min-chunk-size <size>` | the minimum size of chunks (defaults to chunk-size/4) |
| `-zstd-level <level>` | set zstd compression level (fastest, default, better, or best) |
| `-zstd` | short for -zstd default |
| `-iterations <i>` | the number of iterations used in storage key derivation (default is 600000) |
| `-pref-dir <path>` | alternate location for the .duplicacy directory (absolute or relative to current directory) |
| `-storage-name <name>` | assign a name to the storage |
| `-repository <path>` | initialize a new repository at the specified path rather than the current working directory |
| `-key <public key>` | the RSA public key to encrypt file chunks |
| `-erasure-coding <data shards>:<parity shards>` | enable erasure coding to protect against storage corruption |

### backup

Save a snapshot of the repository to the storage

```
duplicacy backup [options]
```

| Option | Description |
|--------|-------------|
| `-hash` | detect file differences by hash (rather than size and timestamp) |
| `-t <tag>` | assign a tag to the backup |
| `-stats` | show statistics during and after backup |
| `-threads <n>` | number of uploading threads |
| `-limit-rate <kB/s>` | the maximum upload rate (in kilobytes/sec) |
| `-dry-run` | dry run for testing, don't backup anything. Use with -stats and -d |
| `-zstd-level <level>` | set zstd compression level (fastest, default, better, or best) |
| `-zstd` | short for -zstd default |
| `-vss` | enable the Volume Shadow Copy service (Windows and macOS using APFS only) |
| `-strict` | exit with code 4 if any file or directory was skipped because of an access error |
| `-vss-timeout <timeout>` | the timeout in seconds to wait for the Volume Shadow Copy operation to complete |
| `-storage <storage name>` | backup to the specified storage instead of the default one |
| `-enum-only` | enumerate the repository recursively and then exit |
| `-metadata-chunk-size <size>` | the average size of metadata chunks (defaults to 1M) |
| `-max-in-memory-entries <number>` | the maximum number of entries kept in memory (defaults to 1M) |

### restore

Restore the repository to a previously saved snapshot

```
duplicacy restore [options] [--] [pattern] ...
```

| Option | Description |
|--------|-------------|
| `-r <revision>` | the revision number of the snapshot (required) |
| `-hash` | detect file differences by hash (rather than size and timestamp) |
| `-overwrite` | overwrite existing files in the repository |
| `-delete` | delete files not in the snapshot |
| `-ignore-owner` | do not set the original uid/gid on restored files |
| `-stats` | show statistics during and after restore |
| `-threads <n>` | number of downloading threads |
| `-limit-rate <kB/s>` | the maximum download rate (in kilobytes/sec) |
| `-storage <storage name>` | restore from the specified storage instead of the default one |
| `-key <private key>` | the RSA private key to decrypt file chunks |
| `-persist` | continue processing despite chunk errors or existing files (without -overwrite), reporting any affected files |
| `-key-passphrase <private key passphrase>` | the passphrase to decrypt the RSA private key |

### list

List snapshots

```
duplicacy list [options]
```

| Option | Description |
|--------|-------------|
| `-all` | list snapshots with any id |
| `-id <snapshot id>` | list snapshots with the specified id rather than the default one |
| `-r <revision>` | the revision number of the snapshot (can be repeated) |
| `-t <tag>` | list snapshots with the specified tag |
| `-files` | print the file list in each snapshot |
| `-chunks` | print chunks in each snapshot or all chunks if no snapshot specified |
| `-reset-passwords` | take passwords from input rather than keychain/keyring |
| `-storage <storage name>` | retrieve snapshots from the specified storage |
| `-key <private key>` | the RSA private key to decrypt file chunks |

### check

Check the integrity of snapshots

```
duplicacy check [options]
```

| Option | Description |
|--------|-------------|
| `-all` | check snapshots with any id |
| `-id <snapshot id>` | check snapshots with the specified id rather than the default one |
| `-r <revision>` | the revision number of the snapshot (can be repeated) |
| `-t <tag>` | check snapshots with the specified tag |
| `-fossils` | search fossils if a chunk can't be found |
| `-resurrect` | turn referenced fossils back into chunks |
| `-rewrite` | rewrite chunks with recoverable corruption |
| `-reverify-days <n>` | with -chunks, verify again chunks last verified more than <n> days ago (default: never) |
| `-files` | verify the integrity of every file |
| `-chunks` | verify the integrity of every chunk |
| `-stats` | show deduplication statistics (imply -all and all revisions) |
| `-tabular` | show tabular usage and deduplication statistics (imply -stats, -all, and all revisions) |
| `-storage <storage name>` | retrieve snapshots from the specified storage |
| `-key <private key>` | the RSA private key to decrypt file chunks |
| `-key-passphrase <private key passphrase>` | the passphrase to decrypt the RSA private key |
| `-threads <n>` | number of threads used to verify chunks |
| `-persist` | continue processing despite chunk errors, reporting any affected (corrupted) files |

### cat

Print to stdout the specified file, or the snapshot content if no file is specified

```
duplicacy cat [options] [<file>]
```

| Option | Description |
|--------|-------------|
| `-id <snapshot id>` | retrieve from the snapshot with the specified id |
| `-r <revision>` | the revision number of the snapshot |
| `-storage <storage name>` | retrieve the file from the specified storage |
| `-key <private key>` | the RSA private key to decrypt file chunks |
| `-key-passphrase <private key passphrase>` | the passphrase to decrypt the RSA private key |

### diff

Compare two snapshots or two revisions of a file

```
duplicacy diff [options] [<file>]
```

| Option | Description |
|--------|-------------|
| `-id <snapshot id>` | diff snapshots with the specified id |
| `-r <revision>` | the revision number of the snapshot (can be repeated) |
| `-hash` | compute the hashes of on-disk files |
| `-storage <storage name>` | retrieve files from the specified storage |
| `-key <private key>` | the RSA private key to decrypt file chunks |
| `-key-passphrase <private key passphrase>` | the passphrase to decrypt the RSA private key |

### history

Show the history of a file

```
duplicacy history [options] <file>
```

| Option | Description |
|--------|-------------|
| `-id <snapshot id>` | find the file in the snapshot with the specified id |
| `-r <revision>` | show history of the specified revisions (can be repeated) |
| `-hash` | show the hash of the on-disk file |
| `-storage <storage name>` | retrieve files from the specified storage |

### prune

Prune snapshots by revision, tag, or retention policy

```
duplicacy prune [options]
```

| Option | Description |
|--------|-------------|
| `-id <snapshot id>` | delete snapshots with the specified id instead of the default one |
| `-all` | match against all snapshot IDs |
| `-r <revision>` | delete snapshots with the specified revisions (can be repeated) |
| `-t <tag>` | delete snapshots with the specified tags (can be repeated) |
| `-keep <n:m>` | keep 1 snapshot every n days for snapshots older than m days (can be repeated) |
| `-exhaustive` | remove all unreferenced chunks (not just those referenced by deleted snapshots) |
| `-ignore-lock` | run even if prune.lock shows that another prune may be in progress |
| `-exclusive` | assume exclusive access to the storage (disable two-step fossil collection) |
| `-dry-run` | show what would have been deleted |
| `-delete-only` | delete fossils previously collected (if deletable) and don't collect fossils |
| `-collect-only` | identify and collect fossils, but don't delete fossils previously collected |
| `-ignore <id>` | ignore snapshots with the specified id when deciding if fossils can be deleted (can be repeated) |
| `-storage <storage name>` | prune snapshots from the specified storage |
| `-threads <n>` | number of threads used to prune unreferenced chunks |

### password

Change the storage password

```
duplicacy password [options]
```

| Option | Description |
|--------|-------------|
| `-storage <storage name>` | change the password used to access the specified storage |
| `-iterations <i>` | the number of iterations used in storage key derivation (default is 600000) |

### add

Add an additional storage to be used for the existing repository

```
duplicacy add [options] <storage name> <snapshot id> <storage url>
```

| Option | Description |
|--------|-------------|
| `-encrypt` | encrypt the storage with a password |
| `-chunk-size <size>` | the average size of chunks (default is 4M) |
| `-max-chunk-size <size>` | the maximum size of chunks (default is chunk-size*4) |
| `-min-chunk-size <size>` | the minimum size of chunks (default is chunk-size/4) |
| `-zstd-level <level>` | set zstd compression level (fastest, default, better, or best) |
| `-zstd` | short for -zstd default |
| `-iterations <i>` | the number of iterations used in storage key derivation (default is 600000) |
| `-copy <storage name>` | make the new storage compatible with an existing one to allow for copy operations |
| `-bit-identical` | (when using -copy) make the new storage bit-identical to also allow rsync etc. |
| `-repository <path>` | specify the path of the repository (instead of the current working directory) |
| `-key <public key>` | the RSA public key to encrypt file chunks |
| `-erasure-coding <data shards>:<parity shards>` | enable erasure coding to protect against storage corruption |

### set

Change the options for the default or specified storage

```
duplicacy set [options]
```

| Option | Description |
|--------|-------------|
| `--encrypt` |  |
| `--no-backup[=true]	backup to this storage is prohibited` |  |
| `--no-restore[=true]	restore from this storage is prohibited` |  |
| `--no-save-password[=true]	don't save password or access keys to keychain/keyring` |  |
| `-nobackup-file <file name>` | Directories containing a file with this name will not be backed up |
| `--exclude-by-attribute[=true]	Exclude files based on file attributes. (macOS only` |  |
| `-key` | add a key/password whose value is supplied by the -value option |
| `-value` | the value of the key/password |
| `-storage <storage name>` | use the specified storage instead of the default one |
| `-filters <file path>` | specify the path of the filters file containing include/exclude patterns |

### copy

Copy snapshots between compatible storages

```
duplicacy copy [options]
```

| Option | Description |
|--------|-------------|
| `-id <snapshot id>` | copy snapshots with the specified id instead of all snapshot ids |
| `-r <revision>` | copy snapshots with the specified revisions (can be repeated) |
| `-from <storage name>` | copy snapshots from the specified storage |
| `-to <storage name>` | copy snapshots to the specified storage |
| `-download-limit-rate <kB/s>` | the maximum download rate (in kilobytes/sec) |
| `-upload-limit-rate <kB/s>` | the maximum upload rate (in kilobytes/sec) |
| `-threads <n>` | number of uploading threads |
| `-download-threads <n>` | number of downloading threads |
| `-key <private key>` | the RSA private key to decrypt file chunks from the source storage |
| `-key-passphrase <private key passphrase>` | the passphrase to decrypt the RSA private key |

### info

Show the information about the specified storage

```
duplicacy info [options] <storage url>
```

| Option | Description |
|--------|-------------|
| `-encrypt` | The storage is encrypted with a password |
| `-repository <repository directory>` | retrieve saved passwords from the specified repository |
| `-storage-name <name>` | the storage name to be assigned to the storage url |
| `-reset-passwords` | take passwords from input rather than keychain/keyring |

### benchmark

Run a set of benchmarks to test download and upload speeds

```
duplicacy benchmark [options]
```

| Option | Description |
|--------|-------------|
| `-file-size <size>` | the size of the local file to write to and read from (in MB, default to 256) |
| `-chunk-count <count>` | the number of chunks to upload and download (default to 64) |
| `-chunk-size <size>` | the size of chunks to upload and download (in MB, default to 4) |
| `-upload-threads <n>` | the number of upload threads (default to 1) |
| `-download-threads <n>` | the number of download threads (default to 1) |
| `-storage <storage name>` | run the download/upload test agaist the specified storage |
