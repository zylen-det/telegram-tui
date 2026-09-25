# Architecture

This document describes the production flow, package boundaries, and key invariants. Live source remains authoritative for exact definitions.

## Production flow

Bubble Tea is the application's only Elm-style state loop:

```text
keyboard / mouse / resize       TDLib updates / effect results
             \                         /
              frontend.AppModel.Update
                   /             \
          frontend.State       tea.Cmd
                 |                 |
          frontend.View       frontend.Handler
                 |                 |
       render-data selection   Telegram / media / platform
                 |
         frontend surfaces
                 |
      modalStack registry
                 |
       one Lipgloss Compositor
                 |
        tea.View + hits + cursor
```

`AppModel.Update` owns the authoritative state transition. Its private feature helpers keep large workflows in separate files, but there is no second Engine, public reducer, executor, or alternate UI event loop. Effect work is scheduled as `tea.Cmd` by Bubble Tea. The Telegram update and authorization prompt streams are each a single subscription: one outstanding wait command at a time, re-armed by `Update` after each stream item. A `Handler` owns the client pump that feeds the update stream.

Kitty image emission and cleanup is a separate overlay lifecycle.

## Package ownership

| Package | Responsibility |
|---|---|
| `cmd/telegram-tui` | Bootstrap, dependencies, process ownership, signals, exit code |
| `internal/domain` | Telegram-independent chat, message, user, media, and error models |
| `internal/frontend` | Bubble Tea model/state/update, effect adaptation, Huh text-input hosts, render-data selection, grouping/layout/hit maps, Lipgloss surfaces/compositor, cursor, and Kitty coordination |
| `internal/frontend/components` | Reusable Lipgloss frontend components |
| `internal/telegram` | Domain-facing client, TDLib adapter, update normalization, safe errors; only this package imports generated TDLib types |
| `internal/media/*` | Avatar, thumbnail, pixel, and Kitty rendering/transport |
| `internal/auth` | Authorization state machine |
| `internal/config` | XDG paths, preferences, credentials, database-key persistence |
| `internal/platform` | Clipboard, external file opening, single-instance locking |
| `internal/logging` | Allow-listed rotating logs |

`internal/buildinfo` remains build support rather than a production entry point.

## Invariants

- Bubble Tea's `AppModel.Update` is the sole application state loop. Do not add another Engine, reducer facade, event bus, command executor, or store around it.
- Each external stream has exactly one outstanding subscription command, re-armed from `Update` after each item. Never issue a fresh wait on every update; do not derive a quit from stream closure while a deliberate shutdown is in progress.
- UI never calls generated TDLib APIs. Effects use the domain-facing `telegram.Client` in `internal/telegram/client.go`; generated TDLib types never leave `internal/telegram`.
- Effect work performs no model mutation. Results and external updates return to `AppModel.Update` as ordinary `tea.Msg` values.
- `AppModel.View` takes one `AppModel.Snapshot`, derives render data with `Select`, and composes one full-size Lipgloss frame.
- Kitty image transport remains outside text composition. The frame describes desired placements; overlay code emits and cleans terminal images.
- Async request results use request and entity identity. Stale results must not overwrite a newer active operation.
- Huh text-input components may own transient editing mechanics; `frontend.State` owns list selection plus durable and business state.
- List modals use the shared manual `modalRowSpec` paint and hit path. Each feature's row builder is the single source for displayed labels, semantic actions, selected-row paint, and mouse identity; section headers and informational rows remain non-interactive. Do not layer a second selector or viewport over these rows.
- Modal composition extends through `defaultModalStack` in `internal/frontend/modal_stack.go`. A modal spec owns activation, toast suppression, rendering adaptation, cursor policy, interactions, and Kitty-inline ownership. `composeApplication` must not grow concrete modal branches.
- Ordinary list keyboard behavior extends through the shared list-focus classification in `internal/frontend/input.go`; do not duplicate close/previous/next/activate switch bodies for each list focus.
- Media main files download only after an explicit open action. Static thumbnails may use the existing shared thumbnail lifecycle.
- Secrets, message text, drafts, raw TDLib payloads, local media paths, and raw errors must not enter logs or user-facing failure messages.

## Useful entry points

- Bubble Tea model/update: `internal/frontend/app_model.go`, `model_state.go`, `update_state.go`, `update_*.go`
- Effects and asynchronous messages: `internal/frontend/effects.go`, `effect_requests.go`, `messages.go`
- Rendering: `internal/frontend/viewmodel.go`, `viewmodel_group.go`, `viewmodel_layout.go`, `render_frame.go`, `modal_stack.go`, `surface_*.go`
- Telegram boundary: `internal/telegram/client.go`, `adapter_tdlib.go`, `normalize_tdlib.go`, `fake.go`
- Product behavior and canonical gates: `README.md`
