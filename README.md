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
> - **Current Windows Store builds** (Windows): finds Claude's app-data in `%LOCALAPPDATA%\Claude-Data\claude_pzs8sxrjxfjjc`.
> - **Account info on Windows** (Windows): `list` shows email and plan, and `use` verifies the restored session with the server — previously macOS-only.
> - **Shared Claude Code sessions** (macOS and Windows): `link-code-sessions` / `unlink-code-sessions` let several accounts show and continue the same sidebar sessions ([details](#shared-claude-code-sessions)).
> - **Usage limits in `list`** (macOS and Windows): new `5H RESET` and `7D RESET` columns show when the 5-hour and 7-day limits reset and how much is used; `EXPIRES` is now `LOGIN EXPIRES`.
> - **Team and Enterprise plans** (macOS and Windows): the `PLAN` column now recognizes them.
> - **One-line Windows installer** (Windows): installs or updates the exe, puts it on your `Path`, and adds the `cdswap` alias ([details](#installation)).
> - **More reliable `add`** (macOS and Windows): if the new login has not reached the disk yet, Claude is relaunched instead of the command failing.
>
> These changes were tested against Claude Desktop on Windows. On macOS they build and pass `go vet`, but have not been run against the app.

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
irm https://raw.githubusercontent.com/robertluedke/claude-desktop-swap/main/install.ps1 | iex
```

[`install.ps1`](install.ps1) downloads the latest release for your CPU (x64 or Arm), installs it to `%LOCALAPPDATA%\Programs\claude-desktop-swap`, adds that folder to your user `Path`, and creates `cdswap` as a short alias. Afterwards both `claude-desktop-swap` and `cdswap` work from any terminal; open a new one if the current one does not find them yet. Run the same line again to update. No admin rights are needed.

### Build from source

```sh
go install github.com/robertluedke/claude-desktop-swap@latest
```

## Usage

```sh
# Save your current session as a named profile (stops and restarts Claude if it is running)
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

Claude Desktop keeps the sidebar's Claude Code sessions per account and organization under `claude-code-sessions/<account>/<org>/`. `link-code-sessions <primary> <profile>...` syncs the session files between the primary's directory and each profile's, right away and again on every `use`, so all linked accounts see and continue the same sessions. For each session the newest copy wins, and deleted sessions are removed everywhere. A backup goes to `~/.claude-swap/sessions-backup-<time>`, and the links are recorded in `~/.claude-swap/session-links.json`.

Every account keeps a real directory. Claude Desktop refuses to save sessions into a junction or symlink, so a session created there exists only in memory and disappears on the next restart (for example after an auto-update). Older versions of this tool linked the directories that way; the next `use`, `link-code-sessions` or `unlink-code-sessions` replaces such a link with a real directory.

`unlink-code-sessions [profile...]` (all linked profiles if none given) runs a last sync and then stops syncing. Each profile gets back exactly the sessions it had before it was linked (recorded at link time); the primary keeps its own plus every session created while linked, whichever account started it.

## First-time setup

1. Make sure you're logged into Claude Desktop as your first account, then snapshot it:
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

`save` stops Claude before snapshotting, because a quiescent database is required for a safe WAL checkpoint, and starts it again afterwards if it was running.

## Profile storage

Profiles are stored at `~/.claude-swap/profiles/<name>/` as a filtered mirror of Claude's app-data tree, alongside a `meta.json` holding non-secret format, identity, health, timestamp, and integrity metadata.

Two denylists decide what never enters a profile:

- **Cache and runtime state**, regenerable on Claude's next launch — `Cache`, `Code Cache`, `GPUCache`, `blob_storage`, `Crashpad`, `logs`, and similar.
- **Machine and in-flight work state**, which must survive an account switch untouched — `ant-did`, `claude_desktop_config.json`, `claude-code-sessions`, `git-worktrees.json`, `window-state.json`, and similar.

Everything else is captured by default, so account state Anthropic adds later travels with the profile instead of silently leaking between accounts.

Directories use `0700`; files use `0600`. Saving and restoring copy files as-is: cookie values are never selected, decrypted, printed, or logged in that path (see [Security](#security) for the account-info exception).

Profiles written by older versions are rejected as an outdated format rather than migrated. Run `save <name>` while signed into that account to re-save it.

Health is based on non-secret local SQLite evidence and is reported as `usable`, `expired`, `missing`, or `unknown`; a profile the server rejects (HTTP 401) is shown as `expired`. Expired, missing, unknown, unsafe, or integrity-mismatched profiles are never restored. Server-side expiry cannot be extended by this tool.

The active profile is tracked at `~/.claude-swap/current`, but `status` identifies the live account by `config.json`'s `lastKnownAccountUuid` rather than by that file or by a cookie digest — the `sessionKey` is a sliding token Claude rotates on its own, so a live digest routinely diverges from what was last saved even for the correct account.

## Usage limits

`list` shows when each account's 5-hour and 7-day usage limits reset, in local time, with the share already used — for example `03:00 (100%)` under `5H RESET` and `Tue 23:00 (37%)` under `7D RESET`. The values are fetched live from `claude.ai` and never stored; `-` means no window is running or the lookup failed (for example offline).

## Session expiry

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

Claude Desktop stores its cookie database under `Network\Cookies` on Windows and at the top level of app-data on macOS; the location is detected at runtime. Three Windows-specific behaviors are worth knowing:

- Current Microsoft Store builds keep their app-data at `%LOCALAPPDATA%\Claude-Data\claude_pzs8sxrjxfjjc`. Older ones are MSIX-packaged, so their real app-data lives at `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude`. `%APPDATA%\Claude` is only a container projection that exists while the app is running. Profiles are captured from the backing store, so they work with Claude stopped. The standalone installer is unpackaged and uses `%APPDATA%\Claude` directly; all are detected automatically.
- Claude Code installs its own `claude.exe` inside the app-data tree. Profile switching matches processes by executable path, not image name, so running CLI sessions are left alone.
- Windows Claude holds the live cookie database open denying all sharing, so its expiry cannot be read while the app runs — `status` shows the active profile but reports the session expiry as `-`. Quit Claude to see it. `save` and `use` are unaffected because they stop the app first.

Account email, plan, and usage limits (`list`, and the post-restore server check in `use`) work on macOS and Windows: the cookie key is read from the macOS keychain, or on Windows unwrapped from `Local State` via DPAPI for the current user.

## Security

Cookie values are encrypted by Chromium with a key held by the OS: the macOS keychain, or on Windows a key protected by DPAPI for the current user. Saving, switching, and `add` never decrypt anything — they copy the raw encrypted files, which are only usable on the same machine (on Windows, for the same Windows user).

The one exception is account info. For email, plan, usage limits, and the post-restore check in `use`, the `sessionKey` cookie is decrypted in memory and sent to `claude.ai` — the only network calls the tool makes, from `list`, the `use` picker, and `use`. The decrypted value is never written to disk, printed, or logged; only email and plan are cached in `meta.json`, and usage limits are not stored at all. These lookups are best-effort: offline, they are skipped, and only an explicit server rejection blocks a switch.

Profile directories are created with `0700` permissions and profile files with `0600`. On macOS, restoration refuses broader permissions. Windows has no POSIX mode bits — `os.FileMode` there is synthesized from file attributes and duplicates the owner bits — so that check is skipped and access is governed by NTFS ACLs, which this tool does not inspect.

## Contributors

- [Francisco Calveyra](https://github.com/FranCalveyra) — original author
- [Robert Lüdke](https://github.com/robertluedke) — Windows Store data path, Windows account info, shared Claude Code sessions, usage-limit columns, Team/Enterprise plan detection, Windows installer
