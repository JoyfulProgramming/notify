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

### bin/serve

Runs the full pipeline in one process and opens the web UI.

```sh
bin/serve
# ingestor :8080 · rule-api :8081 · delivery + web :8082
```

### bin/serve-docs

Serves the interactive HTML plan viewer from `docs/`.

```sh
bin/serve-docs        # opens http://localhost:8000
bin/serve-docs 9000   # custom port
```

### bin/notify

Sends notifications to the local ingestor. Unset flags fall back to `notify-defaults.toml`.

```sh
bin/notify send                          # send with defaults from notify-defaults.toml
bin/notify send -s com.google.gmail      # -s  source app
                -a john@example.com      # -a  account within the app
                -t "New message"         # -t  title
                -b "Hey, are you free?"  # -b  body
                -f alice@example.com     # -f  sent-by (sender)
                -i "thread-42"           # -i  sent-in (thread/channel)
                --source-id msg-001      # original ID in the source system
                --id <uuid>              # provide your own ID (useful for dedup testing)
```

### bin/rules

Manages delivery rules against the local rule API.

```sh
bin/rules create                        # create with defaults from notify-defaults.toml
bin/rules create -s com.google.gmail    # -s  source app (* = wildcard, com.google.* = glob)
                 -a john@example.com    # -a  account
                 -t "*invoice*"         # -t  title pattern

bin/rules list                          # list all active rules
bin/rules delete <ID>                   # delete a rule by ID
```

### bin/test

Runs the full evaluation suite — contract tests and rapid property tests — in-process, no infra needed.

```sh
bin/test                              # run everything
bin/test -run TestContract -v         # just the contract tests
bin/test -run TestProperty -v         # just the property tests
bin/test -rapid.checks=20             # fewer randomised iterations
```

### bin/build

```sh
bin/build   # go build ./...
```

### bin/lint

```sh
bin/lint    # go vet + gofmt check
```

### bin/check-specs

Checks that `specs/` is in sync with `pkg/contracts/`. Intended as a Claude Code stop hook.

```sh
bin/check-specs   # exit 0 = in sync, exit 2 = specs need updating
```

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
