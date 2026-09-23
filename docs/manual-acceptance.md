# Manual acceptance

Run these checks on Arch Linux in Kitty with a dedicated test account. Record only pass/fail, date, environment versions, and sanitized issue references. Never record credentials, phone numbers, usernames, chat text, session files, or avatar files.

## Run metadata

- Date:
- Commit:
- Arch Linux version:
- Kitty version:
- Hyprland version:
- TDLib commit: `49b3bcbb6bfebf2ed44dd9f25102d2e1a94a58c4`
- Result: `NOT RUN | PASS | FAIL`
- Sanitized notes/issues:

## Checklist

- [ ] Fresh API setup, phone verification, optional Telegram 2FA, and restart/session restoration work entirely inside the TUI.
- [ ] Private chats, groups, supergroups, and channels appear with cached and paginated history; real-time updates and chat ordering work.
- [ ] Text send exercises pending, succeeded, failed, and retry states; read-only chats cannot submit.
- [ ] Vim keys, conventional keys, and mouse have parity; wide, normal, narrow, and too-small layouts preserve state.
- [ ] With Kitty window opacity below 1, the terminal background shows through ordinary canvas, pane, input, and modal cells while selected rows and intentional control fills remain legible; authorization and the main application behave consistently.
- [ ] In the Chats pane, `a` opens the focused chat's action modal, while Enter opens the focused conversation and `h`/`l`/Left/Right wrap focus among visible Chats, Conversation, and Info panes without selecting another chat or focusing the message input (`i` focuses the input); keyboard and mouse cover Open chat, View profile/group/channel, Archive, Pin, Mute, Mark as read/unread, and the TDLib-permitted clear/delete/join/leave rows; labels toggle with live state, destructive actions require confirmation, stale results are ignored, archived/deleted/left chats leave the main list, and failures expose no identifiers or raw causes.
- [ ] In the Chats pane, `u` selects the next unread chat and `m` selects the next chat with unread mentions; both wrap, show the matching no-destination toast for current-only/no-match cases, open the selected chat normally, and track live unread/mention count changes.
- [ ] In the Chats pane, `/` opens unified live search; typing filters local chats instantly and shows global messages plus public username results without leaving the input, Up/Down/mouse moves across Chats → Messages → Public chats, Enter opens the chat or jumps to the matched message, exact `@name` still resolves, Esc/q cancels, failures stay sanitized, and existing chats/drafts remain intact.
- [ ] **Draft sync:** create and edit plain-text and reply drafts in the TUI and another Telegram client; drafts restore after chat switching and process restart, remote clean changes appear, stale updates do not replace active local typing, chat rows show bounded `Draft:`/reply-only previews with draft time when available, send/Esc clearing removes the cloud draft, rapid typing settles on the latest value, and failures expose neither draft text nor raw causes.
- [ ] Avatars and `View image` cover expected grouping, loading, ready, retry, close, and cleanup behavior.
- [ ] Local Photo, Video, Audio, and Document selection preserves caption/reply and exercises optimistic pending, success, privacy-safe failure, and retry without changing recognized media kinds.
- [ ] Received and outgoing Photo/Video/Sticker/Document/Animation/Video Note previews have correct Kitty and half-block geometry, captions/metadata/fallbacks, scrolling crop, modal occlusion, and cleanup.
- [ ] Video, Audio, Document, Animation, Voice Note, and Video Note explicit open actions download when needed, launch once through the system application, suppress duplicates/stale results, and never expose a path or raw cause.
- [ ] Sticker receive and picker/send behavior covers static posters, fallback, favorites-before-recents ordering, deduplication, navigation, reply, pending/success/failure, and draft preservation.
- [x] **Inline bot-command completion:** in a private bot chat and a group containing bots, typing `/` at the start of a new-message composer opens the advertised command list directly above the composer; filtering, Up/Down, mouse activation, Enter completion with optional arguments, Esc dismissal, scrolling, chat switching, and narrow geometry work without sending prematurely or affecting edit mode.
- [ ] Main media files never auto-download or play in the terminal; unsupported, secret, spoiler, MPEG4-animation, and Voice Note thumbnail cases retain safe fallback.
- [ ] Cached content remains readable offline and updates after reconnect.
- [ ] Ctrl-C, SIGINT, and SIGTERM shut down within 10 seconds without Kitty artifacts.
- [ ] Logs contain no API hash, phone, code, password, database key, message text, draft, local path, raw error, or raw TDLib payload; local config is mode `0600`.
- [x] **Pinned message views:** `p` opens a reverse-chronological paginated modal in the conversation pane; `j`/`k`/Up/Down navigate, `Enter` loads the selected message in context, `Esc`/`q` closes. Modal renders with correct geometry, dimming/occlusion, and no clickable opener. Pagination loads older results via `fromMessageID` cursor; stale results are rejected by request ID. Live pin/unpin updates reconcile the view. Pinned-specific safe user messages appear on failure.
- [x] **Desktop notifications:** while blurred, new incoming non-service messages from unmuted chats show a bounded chat-title and `Sender: preview` notification. Focused, muted, outgoing, service, and duplicate cases remain silent; unavailable notification services do not interrupt the TUI; previews expose no IDs, paths, raw errors, or unbounded payloads.
- [ ] **Topics:** in a forum supergroup, entering the chat shows the `All messages` merged stream with its own draft and sends to General; `t` opens the paginated topic list where `j`/`k`/Up/Down navigate, `Enter`/mouse selects a topic or `All messages`, and `Esc`/`q` closes without changing topic; The selected topic shows scoped history/composer with `Chat › Topic` title, per-topic drafts/replies, topic-scoped search/pins, and topic-carrying media sends; closed topics render read-only; stale loads never leak into another topic; ordinary chats behave as before.
- [ ] **Members:** in a basic group, supergroup, and channel, the Info pane offers keyboard-selectable `View image` and `Members` rows (`j`/`k`/Up/Down moves, Enter opens, mouse parity, private chats show only `View image`); `m` still opens members directly. One modal pages members in TDLib order with stale-result safety; selecting a member switches the same modal to that user's avatar, info, and actions below (`‹ Back`, view avatar, copy `@username`, add/remove contacts, block/unblock) with sanitized toasts; the avatar modal opens above and returns to the detail on close; `Esc`/`q` backs out to the list, then closes restoring Info focus; any message's action menu offers `User info` for user senders, opening the same single-user view; failures expose no identifiers or raw causes.
- [ ] **Group/channel administration:** in a group and channel where the account may manage links, Info shows `Invite links` and the modal pages active links, creates, copies, and confirms revoke (including primary-link replacement); without that right, the row is absent. In a group with restriction rights, Info shows `Group permissions` and saved default permission toggles persist; channels and unauthorized accounts do not show that row. For a manageable group/channel member, `Manage in chat` opens the member-action modal with only TDLib-permitted actions; promotion/editing rights, demotion, supergroup restriction/unrestriction, removal, and banning produce the expected Telegram result, confirm consequential actions, and refresh the member list. `Esc`/`q`, keyboard/mouse row activation, loading/retry, narrow geometry, stale-result guards, and sanitized failure notices work without leaking link URLs in logs or errors.
- [ ] **Chat settings:** from Info, an authorized group or channel administrator can open `Chat settings` and edit the title and description; an authorized supergroup administrator can also select slow mode (`Off`, 5 seconds, 10 seconds, 30 seconds, 1 minute, 5 minutes, 15 minutes, or 1 hour). Unauthorized accounts and unsupported chat types do not show unavailable rows. Unicode length validation, unchanged-value no-op behavior, keyboard/mouse activation, editor cancel/save, loading/retry, stale-result guards, and sanitized failure notices work without leaking title or description text in logs or errors.

Run the canonical gate block in [`README.md` → Development and verification](../README.md#development-and-verification) before marking PASS.
