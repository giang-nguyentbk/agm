# Antigravity CLI (`agy`) & `agm` Multi-Account Cheatsheet

A concise, step-by-step guide for setting up, managing multiple Google accounts, and running concurrent isolated `agy` CLI sessions.

---

## 1. Prerequisites (Linux)

Install Go compiler:

```bash
sudo snap install go --classic
```

Install OS keyring and D-Bus utilities:

```bash
sudo apt-get update && sudo apt-get install -y libsecret-tools dbus-user-session gnome-keyring
```

Ensure `~/.local/bin` is in your `PATH` (e.g., in `~/.bashrc` or `~/.zshrc`):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Install or update `agm`:

```bash
cd /path/to/agm
go build -o agm .
cp agm ~/.local/bin/agm
```

---

## 2. Managing Accounts & Quotas

### Step 2.1: Initialize the Local Store

```bash
agm init
```

*Data is stored encrypted at `~/.antigravity-agent/` (AES-256-GCM).*

### Step 2.2: Add Google Accounts

Run for each Google account you want to manage:

```bash
agm login
```

*Opens browser for Google OAuth. Repeat for each account.*

### Step 2.3: Set Friendly Aliases

> **Note:** Syntax is `agm alias <alias_name> <email>`.

```bash
agm alias acc1 user1@gmail.com
agm alias acc2 user2@gmail.com
```

To list or delete aliases:

```bash
agm alias              # List all aliases
agm unalias acc1       # Remove an alias
```

### Step 2.4: Check Quotas & Accounts

```bash
agm list               # Summary table of all accounts & remaining quotas
agm info acc1          # Detailed per-model breakdown (Gemini, Claude, GPT)
agm info acc2
```

---

## 3. Workflow 1: Single Session (Sequential Switching)

When using one account at a time:

```bash
# 1. Switch active credential for Antigravity CLI
agm switch acc1 --target agy

# 2. Launch agy with your preferred options
agy --dangerously-skip-permissions

# 3. When quota is depleted or you want to switch accounts:
agm switch acc2 --target agy
agy --dangerously-skip-permissions
```

---

## 4. Workflow 2: Concurrent Multi-Session (Parallel Mode)

### Why is this needed?
By default, `agy` shares a single OS keyring item (`gemini:antigravity`) and a single config directory (`~/.gemini/`). Running two `agy` instances concurrently in regular terminals causes session collisions.

### The Solution: Profile Isolation
Run each account inside an isolated profile under `~/.agy-profiles/<name>` with its own D-Bus session and auto-unlocked keyring.

### Step 4.1: Install the `agy-profile` Helper

Create `~/.local/bin/agy-profile` (run once):

```bash
cat <<'EOF' > ~/.local/bin/agy-profile
#!/usr/bin/env bash
set -e

ACCOUNT="$1"
if [ -z "$ACCOUNT" ]; then
    echo "Usage: agy-profile <account|alias> [agy-flags...]"
    echo ""
    echo "Configured profiles in ~/.agy-profiles/:"
    ls -1 "$HOME/.agy-profiles" 2>/dev/null || echo "  (none)"
    echo ""
    echo "Available accounts in agm:"
    agm list
    exit 1
fi
shift

REAL_HOME="$HOME"
PROFILE_DIR="$REAL_HOME/.agy-profiles/$ACCOUNT"

mkdir -p "$PROFILE_DIR/.runtime" "$PROFILE_DIR/.local/share/keyrings" "$PROFILE_DIR/.gemini/antigravity-cli"
chmod 700 "$PROFILE_DIR/.runtime"

# Ensure agm database and master key are accessible
ln -sfn "$REAL_HOME/.antigravity-agent" "$PROFILE_DIR/.antigravity-agent"

# Copy baseline settings if not present
cp -n "$REAL_HOME/.gemini/antigravity-cli/settings.json" "$PROFILE_DIR/.gemini/antigravity-cli/settings.json" 2>/dev/null || true
cp -n "$REAL_HOME/.gemini/antigravity-cli/jetski_state.pbtxt" "$PROFILE_DIR/.gemini/antigravity-cli/jetski_state.pbtxt" 2>/dev/null || true

# Launch isolated session with auto-unlocked keyring
export PATH="$REAL_HOME/.local/bin:$PATH"
exec env XDG_RUNTIME_DIR="$PROFILE_DIR/.runtime" HOME="$PROFILE_DIR" dbus-run-session -- bash -c '
  eval $(printf "\n" | gnome-keyring-daemon --daemonize --login --components=secrets)
  agm switch "'"$ACCOUNT"'" --target agy
  exec agy "$@"
' -- "$@"
EOF
chmod +x ~/.local/bin/agy-profile
```

### Step 4.2: Launch Concurrent Sessions

Open two independent terminal windows/tabs:

**Terminal 1 (Account 1):**
```bash
agy-profile acc1 --dangerously-skip-permissions
```

**Terminal 2 (Account 2):**
```bash
agy-profile acc2 --dangerously-skip-permissions
```

*Both sessions run simultaneously, completely independent, with zero password prompts.*

---

## 5. Manual Isolation (Without the Helper Script)

If you prefer not using `agy-profile`:

```bash
# Terminal 1:
PROFILE="$HOME/.agy-profiles/acc1"
mkdir -p "$PROFILE/.runtime" "$PROFILE/.local/share/keyrings"
chmod 700 "$PROFILE/.runtime"
ln -sfn "$HOME/.antigravity-agent" "$PROFILE/.antigravity-agent"

XDG_RUNTIME_DIR="$PROFILE/.runtime" HOME="$PROFILE" dbus-run-session -- bash -c "
  eval \$(printf '\n' | gnome-keyring-daemon --daemonize --login --components=secrets)
  agm switch acc1 --target agy
  agy --dangerously-skip-permissions
"

# Terminal 2:
PROFILE="$HOME/.agy-profiles/acc2"
mkdir -p "$PROFILE/.runtime" "$PROFILE/.local/share/keyrings"
chmod 700 "$PROFILE/.runtime"
ln -sfn "$HOME/.antigravity-agent" "$PROFILE/.antigravity-agent"

XDG_RUNTIME_DIR="$PROFILE/.runtime" HOME="$PROFILE" dbus-run-session -- bash -c "
  eval \$(printf '\n' | gnome-keyring-daemon --daemonize --login --components=secrets)
  agm switch acc2 --target agy
  agy --dangerously-skip-permissions
"
```

---

## 6. Common Issues & Quick Fixes

| Problem | Cause | Fix |
|---|---|---|
| `account matching "xyz" not found` | Inverted alias (`email alias` instead of `alias email`) | Run `agm unalias <corrupt_name>` then `agm alias <alias_name> <email>` |
| `install secret-tool (libsecret)` | D-Bus session not reachable or package missing | `sudo apt-get install libsecret-tools` or wrap with `dbus-run-session` |
| Keyring asks for password popup | Keyring daemon needs interactive unlock | Use `printf "\n" \| gnome-keyring-daemon --daemonize --login --components=secrets` |
| Token expired | Token lifetime exceeded | Run `agm validate` or `agm refresh <email>` |
| Both terminals show same email | Shared keyring conflict | Use `agy-profile <alias>` or D-Bus isolation |
