# gnak (Guigui nak) Architecture

This project ports the desktop client experience from [`fiatjaf/vnak`](https://github.com/fiatjaf/vnak) to a Guigui-based UI. The port keeps the nostr-facing logic in Go while replacing the Qt widgets with Guigui widgets rendered by Ebiten.

## Targets to parity with `vnak`

- Secret key entry supporting raw hex, `nsec`, `ncryptsec`, bunker URLs, and inline generation.
- Four functional tabs mirroring vnak:
  - **Event**: compose events, manage tags, publish to relays, copy JSON.
  - **Req**: build filters, send subscriptions, list responses.
  - **Paste**: inspect arbitrary inputs (`npub`, `nevent`, JSON events/filters, nip05, etc.).
  - **Serve**: expose local HTTP helpers (same routes as the Qt version).
- Status line feedback and per-tab asynchronous operations (publish, subscriptions, HTTP server) on top of `fiatjaf.com/nostr/sdk`.

## Project layout

```
cmd/gnak/main.go        Entry point that wires Guigui run options.
internal/app/app.go     High-level application setup and lifetime management.
internal/state/state.go Shared application state (secret key, current tab, status).
internal/ui/root.go     Root widget: secret form, tab bar, content area, status label.
internal/ui/tabs/*.go   One file per tab (event, req, paste, serve) implementing Guigui widgets.
internal/ui/widgets/    Small reusable UI pieces (tab strip, labeled inputs, relay lists, etc.).
internal/paste/         Logic helpers reused by the Paste tab (decode, format output rows).
internal/event/         Event editing helpers (kind metadata, tag handling, JSON encoding).
internal/req/           Filter builder helpers and subscription plumbing.
internal/serve/         HTTP server orchestration reused by UI controls.
```

## Key design decisions

- **Immediate-mode friendly state**: `state.AppState` exposes simple getters/setters guarded by a mutex. Widgets keep only presentation state and ask the shared state for the latest values during `Tick`/event handlers. This keeps the Guigui tree small and deterministic.
- **Asynchronous nostr work**: Publishing, websocket requests, nip05 lookups, and HTTP serving run in background goroutines. They feed results back into `state.AppState` via channels so the UI thread never blocks.
- **Declarative layout helpers**: Common rows/columns (label + input, form sections, tab strip) are implemented once in `internal/ui/widgets` so the four tabs stay concise despite Guigui's manual layout model.
- **Testing hooks**: Non-UI helpers (parser, nostr conversions) live outside of the widget packages, making them testable without spinning up Ebiten.

## Implementation phases

1. **Scaffold** (this change): module init, Guigui entry point, shared state, secret-input header, empty tab placeholders, and Paste tab MVP (decode + display as text rows).
2. **Event + Req tabs**: port composition/filter widgets, hook up nostr SDK for publish/subscription, add relay list management.
3. **Serve tab + background services**: wrap HTTP helper server, expose start/stop controls.
4. **Parity polish**: styling, copy buttons, keyboard shortcuts, error reporting, and additional decoder types.

Each phase keeps the code runnable so Guigui renders a working subset of the UX while we incrementally reach feature parity.
