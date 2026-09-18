# Architecture

This document describes the production flow, package boundaries, and key invariants. Live source remains authoritative for exact definitions.

## Production flow

```text
keyboard / mouse / resize       TDLib updates / command results
             \                         /
              frontend.AppModel.Update
                         |
                  app.Engine.Apply
                         |
                 Reduce(State, Event)
                  /               \
          []app.Command       app.State
                |                 |
        frontend.AppRuntime   Engine.Snapshot
                |                 |
          app.Executor          ui.Select
                |                 |
            app.Handler       ui.ViewModel
                |                 |
 Telegram / media / platform   frontend surfaces
                                  |
                       modalStack registry
                                  |
                         one Lipgloss Compositor
                                  |
                    tea.View + hits + cursor

Kitty image emission and cleanup is a separate overlay lifecycle.
```

## Package ownership

| Package | Responsibility |
|---|---|
| `cmd/telegram-tui` | Bootstrap, dependencies, process ownership, signals, exit code |
| `internal/app` | Authoritative state, actions/events/commands, reducer, engine, executor, handler |
| `internal/domain` | Telegram-independent chat, message, user, media, and error models |
| `internal/frontend` | Production Bubble Tea v2 update/view path, Huh hosts, Lipgloss surfaces/compositor, input, hits, cursor, Kitty coordination |
| `internal/frontend/components` | Reusable Lipgloss frontend components |
| `internal/ui` | Immutable view-model projection, grouping, layout data, hit map, cell metrics; its old gotui renderer is not a production extension point |
| `internal/telegram` | Domain-facing client, TDLib adapter, update normalization, safe errors; only this package imports generated TDLib types |
| `internal/media/*` | Avatar, thumbnail, pixel, and Kitty rendering/transport |
| `internal/auth` | Authorization state machine |
| `internal/config` | XDG paths, preferences, credentials, database-key persistence |
| `internal/platform` | Clipboard, external file opening, single-instance locking |
| `internal/logging` | Allow-listed rotating logs |

Not listed: `internal/testutil` and `internal/buildinfo` provide test and build support; neither is a production entry point.

## Invariants

- Production UI changes target `internal/frontend`, not legacy `ui.Root` or `ui.Runner`.
- UI never calls TDLib directly. Effects go through app commands, `app.Handler`, and the domain-facing `telegram.Client` in `internal/telegram/client.go`.
- TDLib-generated types never leave `internal/telegram`.
- The reducer performs no I/O. Background work returns `app.Event` values; authoritative state changes through `Engine.Apply` on the Bubble Tea update path.
- `AppModel.View` takes one snapshot, `ui.Select` builds an immutable view model, and frontend surfaces compose one full-size Lipgloss frame.
- Kitty image transport remains outside text composition. The frame describes desired placements; overlay code emits and cleans terminal images.
- Async request results use request and entity identity. Stale results must not overwrite a newer active operation.
- Huh components may own transient editing/selection mechanics; `app.State` owns durable and business state.
- List modals use exactly one paint path: the shared Huh selector overlay or manual row paint, never both. `listModalController` derives selector options, authoritative selection, and geometry from the exact displayed/windowed `modalRowSpec` rows; feature-specific parallel option compilers are forbidden. Section headers and other informational rows exist only in manual paint; chat search therefore remains manual.
- Modal composition extends through `defaultModalStack` in `internal/frontend/modal_stack.go`. A modal spec owns activation, toast suppression, rendering adaptation, cursor policy, interactions, and Kitty-inline ownership. `composeApplication` must not grow concrete modal branches.
- Ordinary list keyboard behavior extends through the shared list-focus classification in `internal/frontend/input.go`; do not duplicate close/previous/next/activate switch bodies for each list focus.
- Media main files download only after an explicit open action. Static thumbnails may use the existing shared thumbnail lifecycle.
- Secrets, message text, drafts, raw TDLib payloads, local media paths, and raw errors must not enter logs or user-facing failure messages.

## Useful entry points

- State and effects: `internal/app/state.go`, `event.go`, `command.go`, `reducer.go`, `handler.go`
- Telegram boundary: `internal/telegram/client.go`, `adapter_tdlib.go`, `normalize_tdlib.go`, `fake.go`
- Production UI: `internal/frontend/app_model.go`, `render_frame.go`, `modal_stack.go`, `list_modal_controller.go`, `surface_*.go`
- Projection/layout: `internal/ui/`
- Product behavior and canonical gates: `README.md`
