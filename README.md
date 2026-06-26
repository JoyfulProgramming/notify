# Notify

A working implementation of a notification routing system built to explore [Chad Fowler's Phoenix Architecture](https://aicoding.leaflet.pub/) — the idea that code is disposable cache, not capital. Intent, contracts, and evaluations are the durable layer; the implementation is meant to be deleted and regenerated freely.

Notifications arrive, get matched against user-defined rules, and are delivered to a web stream. No Docker, no cloud — everything runs in a single Go process with an in-memory bus.

## Quick start

```sh
bin/serve          # starts all three services and opens the web UI
bin/notify send    # send a test notification (uses notify-defaults.toml)
bin/rules create   # create a matching rule
bin/rules list     # see active rules
```

## Scripts

| Script | What it does |
|---|---|
| `bin/serve` | Runs the full pipeline in one process: ingestor on `:8080`, rule-api on `:8081`, delivery + web UI on `:8082`. Opens the browser automatically. |
| `bin/serve-docs` | Serves the interactive HTML plan viewer from `docs/` on `:8000`. |
| `bin/notify send` | Sends a notification to the local ingestor. Flags: `-s` (source app), `-a` (account), `-t` (title), `-b` (body), `-f` (sent-by), `-i` (sent-in), `--source-id`, `--id`. Unset flags fall back to `notify-defaults.toml`. |
| `bin/rules create` | Creates a delivery rule via the rule API. Flags: `-s` (source app), `-a` (account), `-t` (title). Wildcards: `*` matches anything, `com.google.*` is a glob. |
| `bin/rules list` | Lists all active rules. |
| `bin/rules delete <ID>` | Deletes a rule by ID. |
| `bin/test` | Runs the full evaluation suite — contract tests and rapid property tests — in-process, no infra needed. Extra args are passed through to `go test`. |
| `bin/build` | Builds all Go packages (`go build ./...`). |
| `bin/lint` | Runs `go vet` and checks `gofmt` formatting. |
| `bin/check-specs` | Checks that `specs/` is in sync with `pkg/contracts/`. Run as a Claude Code stop hook. |

## Rule matching

Each rule field (`source_app`, `source_account`, `title`) accepts:

- `*` — matches any value (wildcard)
- `com.google.*` — glob pattern
- `com.google.gmail` — exact match

A notification is delivered if it matches at least one rule. The rule store enforces that no two rules are subsets or supersets of each other (INV-7).

## Dev defaults

`notify-defaults.toml` sets fallback values for `bin/notify` and `bin/rules` so you can run them without flags during development. Edit it freely.

## Architecture

The pipeline has three logical services, all wired in-process via `pkg/bus`:

1. **Ingestor** (`:8080`) — accepts incoming notifications over HTTP
2. **Rule API** (`:8081`) — CRUD for delivery rules, backed by SQLite
3. **Deliver + Web** (`:8082`) — matches notifications to rules and streams matches to the browser via SSE

The `evaluations/` directory contains the contract and property tests that define correct behaviour. These are the durable artefacts — the implementation in `internal/` is the disposable part.
