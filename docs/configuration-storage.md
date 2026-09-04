# Configuration storage

The gateway uses SQLite as the authoritative persistent configuration store.
The in-memory runtime model remains `config.Config`; SQLite is a persistence
implementation detail and is not queried on each FBS or Shelly request.

## Storage model

- `gateway.db` is authoritative after initialization.
- `config.yaml` is accepted as a legacy first-run import source.
- The original YAML is left untouched during the first import.
- After a successful Admin configuration change, the gateway writes a generated
  YAML compatibility snapshot and moves the previous YAML to `config.yaml.bak`.
- Live relay/status state remains in memory and is not persisted to SQLite.
- TLS keys and certificates remain ordinary protected files; their paths are
  stored in the database.

The SQLite connection is intentionally conservative:

- one database connection
- rollback-journal (`DELETE`) mode
- `synchronous=FULL`
- foreign keys enabled
- 5-second busy timeout
- `PRAGMA quick_check(1)` on open
- schema versioning through `PRAGMA user_version`

The database must be local to the gateway host. Do not place `gateway.db` on
NFS, SMB, or another network filesystem for active/passive sharing. Replicate
configuration between gateway hosts explicitly instead.

## First-run migration

Fresh platform installers select the database path with the
`FBS_GATEWAY_DB_PATH` environment variable and continue invoking the binary with
the legacy `-config` flag. Keeping `-db` off the supervised command line is
intentional: an older binary restored during rollback ignores the environment
variable and can immediately consume the generated YAML compatibility mirror.

On Linux, for example:

```text
FBS_GATEWAY_DB_PATH=/var/lib/fbs-interlock-gateway/gateway.db
fbs-interlock-gateway -config /etc/fbs-interlock-gateway/config.yaml
```

The binary still supports an explicit `-db` flag for manual operation. If an
existing installation receives the new binary through the self-updater before
its service definition is reinstalled, the gateway safely defaults to
`gateway.db` beside the supplied `config.yaml`; that directory is already
writable by the service account. Re-running the installer later moves new
initialization to the preferred platform path through the environment setting.

Startup behavior is deterministic:

1. Open and integrity-check `gateway.db`.
2. Apply any supported database schema migrations.
3. If the database already contains configuration, load it and ignore manual
   edits to `config.yaml`.
4. If the database is uninitialized, load and validate `config.yaml`.
5. Resolve relative TLS paths while loading the YAML.
6. Import the complete configuration in one SQLite transaction.
7. Re-load the imported configuration from SQLite before starting listeners.

If the database is uninitialized and the legacy YAML is absent or invalid, the
gateway refuses to start.

## Admin changes

The existing Admin API remains unchanged. `PUT /api/config` still validates the
complete proposed configuration before accepting it, but persistence is now a
SQLite transaction rather than an authoritative YAML rewrite. A successful save
still requests a process restart so listeners, TLS state, the Shelly transport,
and status structures are rebuilt exactly as before.

Database constraints additionally prevent invalid ports, invalid booleans,
invalid protocols, invalid switch IDs, duplicate listener ports, and invalid
singleton settings from being committed.

## Import and export

Export the authoritative database as YAML:

```text
fbs-interlock-gateway config export \
  -db /var/lib/fbs-interlock-gateway/gateway.db \
  -output fleet.yaml
```

A redacted export for review can be created with:

```text
fbs-interlock-gateway config export \
  -db /var/lib/fbs-interlock-gateway/gateway.db \
  -output fleet-redacted.yaml \
  -redact-secrets
```

Import a complete YAML configuration transactionally:

```text
fbs-interlock-gateway config import \
  -db /var/lib/fbs-interlock-gateway/gateway.db \
  -input fleet.yaml
```

Add `-mirror-config <path>` to `config import` when a generated compatibility
snapshot should also be maintained.

## Installed paths

Recommended production paths are:

### Linux

```text
/opt/fbs-interlock-gateway/fbs-interlock-gateway
/etc/fbs-interlock-gateway/config.yaml            # legacy/rollback mirror
/etc/fbs-interlock-gateway/tls/...
/var/lib/fbs-interlock-gateway/gateway.db          # authoritative
```

### Windows

```text
C:\FBS\fbs-interlock-gateway\fbs-interlock-gateway.exe
C:\FBS\fbs-interlock-gateway\config.yaml          # legacy/rollback mirror
C:\FBS\fbs-interlock-gateway\tls\...
C:\FBS\fbs-interlock-gateway\gateway.db
```

### macOS

```text
/usr/local/libexec/fbs-interlock-gateway/fbs-interlock-gateway
/Library/Application Support/fbs-interlock-gateway/config.yaml
/Library/Application Support/fbs-interlock-gateway/tls/...
/Library/Application Support/fbs-interlock-gateway/gateway.db
```

Standard uninstall preserves the database and TLS identity. Purge removes the
persistent database along with the other preserved application state.

## Security

SQLite changes editability and transactional behavior; it is not encryption.
Shelly passwords stored in `gateway.db` remain recoverable by an account that can
read the database. Protect the database with the same OS-level access controls
used for the existing production configuration. Do not commit production
`gateway.db`, configuration exports containing credentials, or generated
compatibility YAML files to source control.
