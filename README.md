# telegram-tui

A keyboard-and-mouse Telegram client for Kitty on Arch Linux. It uses a quiet rounded-block TUI, deterministic pixel avatars, an explicit original-avatar modal through Kitty Graphics Protocol, and TDLib for one persistent account.

## Capabilities

Implemented and accepted:

- Phone, verification-code, and Telegram 2FA authorization inside the TUI.
- Desktop notifications for new incoming, non-service messages from unmuted chats while the terminal is unfocused, with bounded chat/sender previews and silent fallback when the notification service is unavailable.
- Private chats, basic groups, supergroups, and channels.
- Cached and paginated text history with placeholders for unsupported media.
- Real-time updates, offline cached reading, reconnect status, and plain-text sending.
- Live chat/message updates (open/close selected chat), reaction add/remove reflected immediately.
- Telegram cloud text drafts, including reply targets: drafts restore across restarts/devices, update chat-row previews, clear on send or explicit composer cancellation, and protect active local typing from stale TDLib draft updates.
- Telegram bot-command completion: typing `/` at the start of the composer opens the active bot/group command menu above it; type to filter, use Up/Down or the mouse to select, and press Enter to insert the command for optional arguments.
- Pixel avatars in chat rows and incoming group-message groups.
- Original-avatar modal with explicit Kitty image cleanup.
- Vim-first keys, conventional keys, and mouse parity.
- Wide, normal, narrow, and too-small responsive layouts.
- Terminal-native transparent canvas: ordinary surfaces use the terminal's default background while selected rows and controls retain intentional fills.
- Message actions: reply, copy (system clipboard), user info, edit, delete, forward, pin, and reactions (capability-aware).
- Centralized action menu, auto-expiring toast, rounded selection card, and per-message metadata.
- Single-instance startup (second process safely rejected).
- **Media send/receive:** local Photo, Video, or Audio send via the shared `[Photo]` control or `Ctrl+O`, with caption, reply target, optimistic pending/failed states, retry, and stale-result safety. Received Photos show inline Kitty previews with half-block fallback and open in the existing `View image` / Kitty modal. Received and outgoing Videos show a static poster when their thumbnail is reachable, plus available filename/duration/dimensions/MIME metadata; `Open video` downloads the main file when needed and launches the system external player. Received and outgoing Audio show caption plus available filename/duration/MIME metadata; `Open audio` likewise downloads when needed and launches the system external player. Received and outgoing Stickers show a static inline poster (including animated/video Stickers), `[Sticker]` fallback, and a scrollable `[Sticker]`/`Ctrl+S` picker (favorites then recents, deduplicated, thumbnail tiles with emoji fallback, Kitty/half-block lifecycle). Selecting a Sticker sends with reply support, optimistic pending/success, and retry.

Received and historical outgoing Documents, Animations/GIFs, Voice Notes, and Video Notes show caption plus a stable one-row metadata tag (`[File]`, `[Animation]`, `[Voice note]`, `[Video note]`) and available filename/duration/dimensions/size/MIME; available Document, static Animation, and Video Note thumbnails show a static inline poster through the Kitty/half-block lifecycle, with Voice Note and MPEG4/secret/spoiler thumbnails falling back to metadata. Each exposes `Open file`, `Open animation`, `Open voice note`, or `Open video note`, which explicitly downloads the main file when needed and launches the system-default external application. The existing `Ctrl+O` / `[Photo]` chooser also sends an ordinary non-Photo/Video/Audio path as a Document with caption, reply, optimistic pending/failed state, and retry.

Implemented and awaiting manual acceptance:

- **Chat action modal:** pressing Enter on the selected chat opens an official-style, capability-aware action list. It provides Open chat, View profile/group/channel, Archive/Unarchive, Pin/Unpin, Mute/Unmute, Mark as read/unread, and—when TDLib permits—Clear history, Delete conversation/group/channel, or Join/Leave. Destructive actions require a separate confirmation; results are request-correlated and failures stay sanitized.
- **Unified global search** from the Chats pane: type `/` to open the search input, then type a query to see live results across three sections (Chats, Messages, Public chats): instant local-chat filtering, global messages across all chats, and public chats by username. The input lives inside the modal and stays focused while typing; results are grouped under Chats / Messages / Public chats headers with dividers. Up/Down or mouse selects across the flattened list, Enter opens the chat or jumps to the matched message, Esc closes.
- **Draft sync:** supported plain-text drafts and same-chat reply targets are loaded from Telegram and synchronized as the composer changes. Remote changes update inactive/clean drafts, while stale updates cannot overwrite active local edits. Chat rows show `Draft:` previews, reply-only drafts use a safe label, and sending or explicitly clearing the composer removes the cloud draft. Formatting entities, effects, suggested posts, and voice/video-note drafts are not editable in this client.
- **Topics:** forum-topic browse, read, and send. Entering a forum chat shows the `All messages` merged stream (sends go to the General topic with an independently stored draft); `t` opens the paginated topic list, and selecting a topic opens topic-scoped history and composer (`Chat › Topic` title) with per-topic drafts, replies, search, pinned messages, and media sends. `t` reopens the list, `Esc` closes it without changing topic, closed topics stay readable but not writable, and topic creation/administration stays out of scope.
- **Members:** member list for basic groups, supergroups, and channels from the Info pane, where `View image` and `Members` are keyboard-selectable (`j`/`k`/Up/Down moves, Enter opens) with mouse parity. The single modal shows each visible member's display name, optional `@username`, and owner/administrator/restricted role with custom title when available; results page in TDLib order with stale-result safety. Selecting a member switches the same modal to that user's avatar, info, and actions below (`‹ Back`, view avatar in the original-image modal, copy `@username`, add/remove contacts, block/unblock user), each with sanitized success/failure toasts. Any message's action menu also offers `User info`, opening the same single-user view (avatar, info, actions) for that sender. `Esc`/`q` backs out to the list, then closes, and failures never leak identifiers or raw causes.
- **Group/channel administration:** administrators with the required TDLib rights can open `Invite links` from Info to page through their active links and the primary link, create an ordinary link, copy its URL, or revoke it after confirmation (revoking the primary link replaces it). Group administrators with restriction rights can edit default member permissions from Info. In the member detail modal, authorized administrators can promote or edit an editable administrator's rights, demote, restrict or unrestrict supergroup members, and remove or ban members; role changes refresh the member list. Authorized administrators can also edit group/channel titles and descriptions, and set supergroup slow mode to Telegram's supported delays. Rights and permissions are loaded from TDLib before editing, actions are gated by the current user's rights, and stale results and failures remain contained in the modal. Named/expiring/join-request invite-link creation, join-request moderation, timed restrictions, and topic administration remain for later work.

Not yet implemented (roadmap items, not rejected): topic administration, multi-account, calls, stories, and non-Kitty terminals.

## Platform and prerequisites

The supported release target is Arch Linux x86-64 with Kitty. The TUI always uses the terminal's default background; actual window translucency is controlled by Kitty (for example, `background_opacity 0.9` in `kitty.conf`) and is not an in-app setting. Desktop notifications require an active freedesktop-compatible notification daemon on the session D-Bus (for example, Mako, Dunst, or SwayNotificationCenter).

Create a personal Telegram application at <https://my.telegram.org/apps> to obtain `api_id` and `api_hash`.

## Install a release

Install the latest Linux x86-64 release without `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/zylen-det/telegram-tui/main/scripts/install.sh | sh
```

The installer verifies the release checksum, installs versioned files under `~/.local/opt/telegram-tui`, and links `~/.local/bin/telegram-tui`. Ensure `~/.local/bin` is in `PATH`. To select a release or another prefix:

```bash
curl -fsSL https://raw.githubusercontent.com/zylen-det/telegram-tui/main/scripts/install.sh \
  | VERSION=v0.1.0 PREFIX="$HOME/.local" sh
```

Alternatively, download the Linux x86-64 archive and `SHA256SUMS` from the [GitHub Releases](https://github.com/zylen-det/telegram-tui/releases) page, verify it, and extract it anywhere:

```bash
sha256sum --check SHA256SUMS
tar -xzf telegram-tui_<version>_linux_x86_64.tar.gz
./telegram-tui_<version>_linux_x86_64/telegram-tui
```

The archive includes the matching TDLib shared library and all required license notices. Keep its `lib` directory beside the executable.

## Build from source

Install the Go/CGO toolchain and runtime dependencies:

```bash
sudo pacman -S --needed base-devel openssl zlib go git curl kitty
```

The default setup downloads the pinned Linux x86-64 TDLib binary from [`eilvelia/tdl`'s `prebuilt-tdlib`](https://github.com/eilvelia/tdl/tree/main/packages/prebuilt-tdlib), verifies fixed archive and library checksums, and installs it under `.local/tdlib` without `sudo`:

```bash
make tdlib
make build
./bin/telegram-tui
```

To build the same pinned TDLib commit from source instead, install `cmake` and `gperf`, then use the fallback target:

```bash
sudo pacman -S --needed cmake gperf
make tdlib-source
make build
```

Check a binary without opening the terminal UI:

```bash
./bin/telegram-tui --version
```

## Command-line options

Run `telegram-tui` without arguments to start the client. Credentials are accepted only through the TUI, environment, or local config.

| Option | Action |
|---|---|
| `-h`, `--help` | Show command-line help and exit |
| `-v`, `--version` | Show the version and exit |

## Credentials and first run

Credential priority is fixed:

| Value | Priority |
|---|---|
| `api_id` | `TELEGRAM_API_ID` → config file → first-run TUI prompt |
| `api_hash` | `TELEGRAM_API_HASH` → config file → first-run TUI prompt |
| TDLib database key | Config file; generated locally if absent |

A prompted `api_id`, `api_hash`, and the random TDLib database encryption key are saved in `config.toml`. Its directory is mode `0700` and the file is mode `0600`. Environment variables are supported for one launch but are not copied to disk.

## Controls

| Action | Vim | Conventional | Mouse |
|---|---|---|---|
| Move selection | `j` / `k` | Up / Down | Row click or wheel |
| Move focus | `h` / `l` | Shift-Tab / Tab | Pane click |
| Open chat action modal (Chats pane) | Enter | Enter | — |
| Open/activate | Enter | Enter | Action click |
| Toggle details | `i` | F2 | Info action |
| View members | `m` or `j`/`k` + Enter in Info pane | Up/Down + Enter | `Members` action |
| Open forum topics | `t` in conversation, Enter in forum chat | Enter in forum chat | Topic row click |
| Member detail | Enter on member (`j`/`k` to move) | Enter | Member row click |
| Unified global search | `/` in Chats pane | `/` in Chats pane | Search result click |
| Close/back | Esc; `q` in modal | Esc | Close/outside action |
| Page history | Ctrl-u / Ctrl-d | Page Up / Page Down | Conversation wheel |
| Send | Enter in composer | Enter | Send action |
| Complete bot command | — | Type `/`, then Up / Down and Enter | Command row click |
| Send local Photo, Video, Audio, or Document | `Ctrl+O` (in composer) | — | `[Photo]` control |
| Open Sticker picker | `Ctrl+S` (in composer) | — | `[Sticker]` control |
| Newline | Shift-Enter | Shift-Enter | — |
| Quit process | Ctrl-C | Ctrl-C | — |

Plain `q` does not quit the process; it closes the active modal/page.

## Responsive layout

- **Wide:** at least 120×24; chat and conversation, optional pushed-in details.
- **Normal:** 80–119 columns and at least 20 rows; details is a full content page.
- **Narrow:** 60–79 columns and at least 18 rows; chat, conversation, and details are separate pages.
- **Too small:** under 60 columns or 18 rows; a size message is shown while state is retained.

## Local data

| Data | Default path |
|---|---|
| Preferences | `~/.config/telegram-tui/config.toml` |
| State/log | `~/.local/state/telegram-tui/` |
| TDLib log | `~/.local/state/telegram-tui/tdlib.log` (10 MiB rotating cap) |
| TDLib database | `~/.local/share/telegram-tui/tdlib/database/` |
| TDLib avatar files | `~/.cache/telegram-tui/avatars/files/` |
| Disposable pixel cache | `~/.cache/telegram-tui/avatars/pixels/` |

XDG environment variables override their corresponding roots. Deleting the avatar cache does not affect the Telegram session. Shutdown asks TDLib to close and preserves its encrypted local session.

Logs are JSON, rotating, and restricted to an allow-list of operation, normalized kind, chat ID, duration, and count. They never include credentials, message/draft text, errors' raw causes, commands, events, or Telegram payloads.

## Development and verification

Default tests never require real Telegram credentials, Kitty, or native TDLib.

```bash
make verify
make tdlib
make test-tdlib
make build
./bin/telegram-tui --version
git diff --check
```

Manual acceptance is tracked in [`docs/manual-acceptance.md`](docs/manual-acceptance.md), and the package boundaries are documented in [`docs/architecture.md`](docs/architecture.md).

## Release process

Tags matching `v*` run the release workflow, repeat the full verification and TDLib integration tests, and publish a checksummed Linux x86-64 archive. A release can be reproduced locally with:

```bash
make tdlib
make package VERSION=v0.1.0
sha256sum --check dist/SHA256SUMS
```

## License

telegram-tui is available under the [MIT License](LICENSE). Bundled release archives include the licenses for TDLib, the prebuilt package, and Go dependencies.

## Roadmap order

Implemented: message actions; official-style chat-list action modal; Photo, Video, Audio, Sticker, and generic attachment receive/send/open flows; unified global search from Chats pane (local-chat filter, public-chat, and global message results); current-chat search; unread/mention navigation; pinned-message views; desktop notifications; inline bot-command completion; cloud text draft sync; group/channel member list and core administration; and forum-topic browse/read/send. The chat action modal, global search, draft sync, members, administration, and topics still await manual acceptance.

Next roadmap item: richer group/channel controls supported by TDLib, including link options, join requests, timed restrictions, and topic administration.

Remaining roadmap, in order (each requires a separate design + implementation decision; no promised dates):

1. Groups/channels: named/expiring/join-request links, join-request moderation, timed restrictions, topic administration, and other management controls beyond the implemented core administration.
2. Platform expansion: multiple accounts, other terminal protocols, macOS, and migrations.
