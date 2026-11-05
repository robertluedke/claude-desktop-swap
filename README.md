<div align="center">
  <img src="assets/claude-desktop-swap-512.png" width="160" height="160" alt="claude-desktop-swap icon">

  # claude-desktop-swap

  [![CI](https://github.com/robertluedke/claude-desktop-swap/actions/workflows/ci.yml/badge.svg)](https://github.com/robertluedke/claude-desktop-swap/actions/workflows/ci.yml)
  [![Release](https://img.shields.io/github/v/release/robertluedke/claude-desktop-swap)](https://github.com/robertluedke/claude-desktop-swap/releases/latest)
  [![Go version](https://img.shields.io/github/go-mod/go-version/robertluedke/claude-desktop-swap)](go.mod)
  [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
</div>

Switch between multiple Claude Desktop accounts without logging out of any of them.

> **This is an updated fork** of [FranCalveyra/claude-desktop-swap](https://github.com/FranCalveyra/claude-desktop-swap) with new features and fixes:
>
> - **Current Windows Store builds:** finds Claude's app-data in `%LOCALAPPDATA%\Claude-Data\claude_pzs8sxrjxfjjc`.
> - **Shared Claude Code sessions:** `link-code-sessions` / `unlink-code-sessions` let several accounts show and continue the same sidebar sessions ([details](#shared-claude-code-sessions)).
> - **Account info on Windows:** `list` shows email and plan on Windows too, including Team and Enterprise plans, and `use` verifies the restored session with the server.
> - **Usage limits in `list`:** new `5H RESET` and `7D RESET` columns show when the 5-hour and 7-day limits reset and how much is used; `EXPIRES` is now `LOGIN EXPIRES`.
> - **More reliable `add`:** if the new login has not reached the disk yet, Claude is relaunched instead of the command failing.

## How it works

Claude Desktop is an Electron app whose whole app-data directory carries the signed-in state — the `Cookies` SQLite database, Local Storage, IndexedDB, Session Storage, and `config.json`. `claude-desktop-swap` stores a filtered mirror of that entire tree as a named profile and swaps it only after Claude and its helper processes have fully stopped.

Before restoring the incoming profile, a switch checkpoints the currently tracked outgoing profile. This preserves the cookie refreshes Claude made since the previous switch.

## Installation

Download the latest binary for your platform from the [releases page](../../releases/latest).

**macOS (Apple Silicon)**
```sh
curl -L https://github.com/robertluedke/claude-desktop-swap/releases/latest/download/claude-desktop-swap_darwin_arm64.tar.gz | tar xz
sudo mv claude-desktop-swap /usr/local/bin/
```

**macOS (Intel)**
```sh
curl -L https://github.com/robertluedke/claude-desktop-swap/releases/latest/download/claude-desktop-swap_darwin_amd64.tar.gz | tar xz
sudo mv claude-desktop-swap /usr/local/bin/
```

**Windows (PowerShell)**
```powershell
$dir = "$env:LOCALAPPDATA\Programs\claude-desktop-swap"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest -Uri https://github.com/robertluedke/claude-desktop-swap/releases/latest/download/claude-desktop-swap_windows_amd64.zip -OutFile "$env:TEMP\cds.zip"
Expand-Archive -Path "$env:TEMP\cds.zip" -DestinationPath $dir -Force
[Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$dir", "User")
```

Open a new terminal afterwards so the updated `Path` takes effect. On Arm devices substitute `claude-desktop-swap_windows_arm64.zip`.

### Build from source

```sh
go install github.com/robertluedke/claude-desktop-swap@latest
```

## Usage

```sh
# Save your current session as a named profile (quit Claude first)
claude-desktop-swap save personal

# Add another account interactively — no manual logout required
claude-desktop-swap add work

# Switch to a saved profile (kills and restarts Claude Desktop)
claude-desktop-swap use work

# List all saved profiles (* = active)
claude-desktop-swap list

# Show the currently active profile and how long its session has left
claude-desktop-swap status

# Same, but report through the exit code: 0 healthy, 1 expiring soon, 2 expired
claude-desktop-swap status --check

# Delete a profile
claude-desktop-swap delete old-account

# Show work's Claude Code sessions in personal too, and continue them there
claude-desktop-swap link-code-sessions work personal

# Give personal its own Claude Code sessions again
claude-desktop-swap unlink-code-sessions personal
```

### Shared Claude Code sessions

Claude Desktop keeps the sidebar's Claude Code sessions per account and
organization under `claude-code-sessions/<account>/<org>/`. `link-code-sessions
<primary> <profile>...` merges each profile's sessions into the primary's
directory and replaces theirs with a link (a junction on Windows), so all
linked accounts see and continue the same sessions — nothing is copied. A
backup goes to `~/.claude-swap/sessions-backup-<time>`, and the links are
recorded in `~/.claude-swap/session-links.json`.

`unlink-code-sessions [profile...]` (all linked profiles if none given) removes
the link. Sessions a profile brought in when it was linked move back to it;
sessions created while linked stay with the primary. Cowork sessions are not
affected. Connectors bound to the primary's organization are not available
when continuing a session from another account.

## First-time setup

1. Make sure you're logged into Claude Desktop as your first account. Quit Claude Desktop, then snapshot it:
   ```sh
   claude-desktop-swap save personal
   ```
2. Add a second account — `add` snapshots `personal`, clears the slate, opens Claude for you to log in, and saves the new session:
   ```sh
   claude-desktop-swap add work
   ```
3. Repeat `add <name>` for any additional accounts.

From here on, switching is one command:

```sh
claude-desktop-swap use personal
claude-desktop-swap use work
```

> **Important:** Never manually log out of Claude Desktop to set up a new account — Anthropic invalidates the session server-side and the snapshot becomes useless. Always use `add` or `save` to capture a session **before** any logout.

`save`, including `save --force`, refuses to snapshot while Claude is running. A quiescent database is required for a safe WAL checkpoint.

## Profile storage

Profiles are stored at `~/.claude-swap/profiles/<name>/` as a filtered mirror of Claude's app-data tree, alongside a `meta.json` holding non-secret format, identity, health, timestamp, and integrity metadata.

Two denylists decide what never enters a profile:

- **Cache and runtime state**, regenerable on Claude's next launch — `Cache`, `Code Cache`, `GPUCache`, `blob_storage`, `Crashpad`, `logs`, and similar.
- **Machine and in-flight work state**, which must survive an account switch untouched — `ant-did`, `claude_desktop_config.json`, `claude-code-sessions`, `git-worktrees.json`, `window-state.json`, and similar.

Everything else is captured by default, so account state Anthropic adds later travels with the profile instead of silently leaking between accounts.

Directories use `0700`; files use `0600`. Cookie values are never selected, decrypted, printed, or logged.

Profiles written by older versions are rejected as an outdated format rather than migrated. Run `save <name>` while signed into that account to re-save it.

Health is based on non-secret local SQLite evidence and is reported as `usable`, `expired`, `missing`, or `unknown`. Expired, missing, unknown, unsafe, or integrity-mismatched profiles are never restored. Server-side expiry cannot be extended by this tool.

The active profile is tracked at `~/.claude-swap/current`, but `status` identifies the live account by `config.json`'s `lastKnownAccountUuid` rather than by that file or by a cookie digest — the `sessionKey` is a sliding token Claude rotates on its own, so a live digest routinely diverges from what was last saved even for the correct account.

## Session expiry

`list` also shows when the 5-hour and 7-day usage limits reset (`5H RESET`, `7D RESET`, with the share already used), fetched live from claude.ai.

`list`, `status`, and the `use` picker show a `LOGIN EXPIRES` column derived from the `sessionKey` cookie's own expiry, and `save`/`use` print a warning when a session has 7 days or less left. The `sessionKey` is a sliding ~28-day token that Claude renews whenever you use that account, so reaching the 7-day window means the profile has gone roughly three weeks untouched.

Expiry is advisory, never a block: a session expiring tomorrow is still perfectly usable and still switches. A profile shows `-` when no deadline is known — a non-persistent cookie or an unreadable database — and no warning is invented from missing data. When the server has already rejected a session, that verdict wins over whatever the local cookie claims.

`status --check` exits `0` when healthy, `1` when the session expires within 7 days, and `2` when it has expired, so it can drive a shell prompt or a cron job.

## Switch safety and preserved data

A switch follows: target preflight → verified full stop → outgoing WAL checkpoint and atomic profile commit → staged incoming replacement → volatile-cache clearing → active tracking → launch. Interrupted profile writes retain the previous generation for recovery. A failed incoming replacement rolls back live Cookies and does not report success. If launch fails after commit, the incoming profile remains active and Claude can be opened manually.

Replacement operates on whole top-level app-data entries: every entry that isn't denylisted is moved into a same-directory rollback backup, the profile is copied in its place, and the backup is discarded only once the copy has committed cleanly. Denylisted entries are never read, replaced, or removed, so machine and work state stays put across a switch.

## Platform support

| OS | Status |
|----|--------|
| macOS | Supported |
| Windows | Supported |

Claude Desktop stores its cookie database under `Network\Cookies` on Windows and at the top level of app-data on macOS; the location is detected at runtime. Three Windows-specific behaviours are worth knowing:

- Current Microsoft Store builds keep their app-data at `%LOCALAPPDATA%\Claude-Data\claude_pzs8sxrjxfjjc`. Older ones are MSIX-packaged, so their real app-data lives at `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude`. `%APPDATA%\Claude` is only a container projection that exists while the app is running. Profiles are captured from the backing store, so they work with Claude stopped. The standalone installer is unpackaged and uses `%APPDATA%\Claude` directly; both are detected automatically.

- Claude Code installs its own `claude.exe` inside `%APPDATA%\Claude`. Profile switching matches processes by executable path, not image name, so running CLI sessions are left alone.
- Windows Claude holds the live cookie database open denying all sharing, so its expiry cannot be read while the app runs — `status` shows the active profile but reports the session expiry as `-`. Quit Claude to see it. `save` and `use` are unaffected because they stop the app first.

Account email and plan (`list`, and the post-restore server check in `use`) work on macOS and Windows: the cookie key is read from the macOS keychain, or on Windows unwrapped from `Local State` via DPAPI for the current user.

## Security

Cookie values are encrypted by Chromium using your OS keychain. `claude-desktop-swap` never decrypts them — it copies raw encrypted blobs, which are only usable on the machine where they were created.

Profile directories are created with `0700` permissions and profile files with `0600`. On macOS, restoration refuses broader permissions. Windows has no POSIX mode bits — `os.FileMode` there is synthesized from file attributes and duplicates the owner bits — so that check is skipped and access is governed by NTFS ACLs, which this tool does not inspect.

## Contributors

- [Francisco Calveyra](https://github.com/FranCalveyra) — original author
- [Robert Lüdke](https://github.com/robertluedke) — Windows Store data path, shared Claude Code sessions, Windows account info, usage-limit columns
