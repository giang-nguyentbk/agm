# agm

Multi-account CLI for [Antigravity](https://antigravity.google/) products:

- **Antigravity CLI** (`agy`)
- **Antigravity IDE**
- **Antigravity** desktop

Manage Google accounts, quotas, and session switching from a single static binary. Own encrypted local store — no other apps required for login or account management.

**Module:** [`github.com/shyim/agm`](https://github.com/shyim/agm)

---

## Install

```bash
go install github.com/shyim/agm@latest
```

Or from a clone of this repository:

```bash
go build -o agm .
# optional: install to PATH
cp agm ~/.local/bin/agm
```

Requires **Go 1.22+** to build. Runs on **macOS**, **Linux**, and **Windows**.

```bash
agm help
```

### Environment

| Variable | Purpose |
|----------|---------|
| `AGM_DATA_DIR` | Data directory (default `~/.antigravity-agent`) |
| `AGM_DB_PATH` | Full path to `cloud_accounts.db` |
| `ANTIGRAVITY_AGENT_DIR` | Alias for `AGM_DATA_DIR` |

---

## Quick start

```bash
agm init                          # create local store (also automatic)
agm login                         # add a Google account via browser OAuth
agm list
agm switch you@gmail.com --target agy
```

---

## Adding accounts

### Browser OAuth (recommended)

```bash
agm login
# alias: agm add
```

1. Listens on `http://localhost:8888` (falls back through `8892` if busy)
2. Opens the browser for Google consent (offline access + refresh token)
3. Stores encrypted tokens and a quota snapshot locally

Run once per Google account.

### Import an existing Antigravity session

| Command | Source |
|---------|--------|
| `agm import-cli` | OS credential store used by **agy** |
| `agm import-ide` | Antigravity IDE `state.vscdb` |

Imports the **currently active** session only. For additional accounts, use `agm login` or switch the product session and import again.

### Backup / restore

```bash
agm export accounts_backup.json
agm import-backup accounts_backup.json
```

Exports keep fields **encrypted**. Restoring on another machine needs the same master key (`.mk`). Prefer `agm login` on a fresh machine.

---

## Switching accounts

```bash
agm switch <email|alias|partial> [--target agy|ide|all]
```

| Target | Product | Behavior |
|--------|---------|----------|
| `agy` (`cli`) | Antigravity CLI | Write OS credential store only (no GUI process restart) |
| `ide` | Antigravity IDE | Inject OAuth into `state.vscdb`, restart IDE if found |
| `all` (default) | All surfaces | Applies `agy` → `ide` |

```bash
agm switch work --target agy
agm switch you@gmail.com -t ide
agm switch personal --target=all
```

Expired access tokens are refreshed automatically when a refresh token is available.

### Aliases

```bash
agm alias work you@gmail.com
agm switch work --target agy
agm alias          # list
agm unalias work
```

---

## Everyday usage

```bash
agm list                       # accounts + quota summary (tags: cli / ide)
agm info you@gmail.com         # per-model quotas
agm status                     # active account snapshot
agm refresh you@gmail.com
agm refresh-all
agm validate                   # refresh expired tokens only
agm sync                       # local DB vs IDE vs CLI credentials
agm doctor                     # diagnostics
agm watch --interval 15        # live list
agm remove you@gmail.com
agm auto-switch --min 40 --model claude
```

---

## Data directory

Default: **`~/.antigravity-agent/`**

```plaintext
~/.antigravity-agent/
├── cloud_accounts.db    # SQLite (encrypted token/quota fields + settings)
├── .mk                  # AES-256 master key as hex (mode 0600)
└── aliases.json         # optional shortcuts
```

Created automatically on first use.

---

## How switching works

```plaintext
  agm login / import-*
           │
           ▼
   ~/.antigravity-agent/
     cloud_accounts.db
           │
           │  agm switch --target …
     ┌─────┴─────┐
     ▼           ▼
    agy         IDE
  keychain     vscdb
```

- **Credential store** — macOS Keychain (`gemini` / `antigravity`), Linux `secret-tool`, Windows Credential Manager
- **SQLite** — unified OAuth entry in Antigravity `state.vscdb` (`antigravityUnifiedStateSync.oauthToken`)

---

## Platform notes

| OS | Credential store |
|----|------------------|
| macOS | Keychain via `security` |
| Linux | `secret-tool` (libsecret) |
| Windows | Credential Manager (`gemini:antigravity`) |

Restarting IDE/desktop after `switch` is best-effort. Token injection still applies if the executable is not found.

---

## Security

- Tokens encrypted at rest with **AES-256-GCM** (`agm_enc_v1:…`)
- Master key in `.mk` — keep `~/.antigravity-agent` private (`0700` / `0600`)
- Do not commit `.mk`, databases, or export files
- Uses the public Antigravity OAuth client (same as the official apps)

---

## Command reference

| Command | Description |
|---------|-------------|
| `init` | Create data dir, key, and empty DB |
| `login` / `add` | Browser OAuth |
| `import-cli` / `import-agy` | Import from agy credential store |
| `import-ide` | Import from IDE database |
| `list` / `ls` | List accounts |
| `info` / `status` | Details / snapshot |
| `switch` | Apply account to product surface(s) |
| `refresh` / `refresh-all` / `validate` | Tokens & quotas |
| `sync` / `doctor` | Status & diagnostics |
| `remove` / `rm` | Delete local account |
| `alias` / `unalias` | Shortcuts |
| `export` / `import-backup` | Backup |
| `auto-switch` | Best quota account |
| `watch` | Live monitoring |
| `help` | Help |

---

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Missing database / master key | `agm init` or `agm login` |
| OAuth never completes | Free ports `8888`–`8892`; allow localhost |
| `import-cli` fails | Sign into `agy` once, or use `agm login` |
| IDE switch finds no DB | Open Antigravity IDE once |
| Empty quotas | `agm refresh <email>` |
| Expired tokens | `agm validate` |

```bash
agm doctor
```

---

## Development

```bash
go test ./...
go vet ./...
go build -o agm .
```

```plaintext
.
├── main.go
├── go.mod                 # module github.com/shyim/agm
├── internal/
│   ├── api/               # OAuth, refresh, quota
│   ├── credstore/         # OS credential store
│   ├── crypto/            # master key + AES-GCM
│   ├── db/                # SQLite + IDE inject
│   ├── paths/
│   ├── process/
│   ├── proto/
│   ├── target/            # agy | ide | all
│   └── aliases/
└── README.md
```

---

## License

See [LICENSE](./LICENSE) in this repository.
