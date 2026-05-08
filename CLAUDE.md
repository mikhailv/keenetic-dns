# CLAUDE.md

This file provides guidance for AI assistants working with this codebase.

## Project Overview

**keenetic-dns** is a DNS server with selective routing capabilities designed for Keenetic routers. It resolves specific domains (YouTube, Facebook, Instagram, etc.) and automatically adds IP routes to route their traffic through configured interfaces (VPN tunnels) while other traffic uses the default route.

## Architecture

```
Client → DNS Server → Upstream Providers (DoH/DNS)
                    │
                    ├→ IP Routing → Agent (on Keenetic) → ip route / ip rule
                    ├→ Cache (in-memory)
                    └→ Storage (persistent file storage)
```

## Key Components

### dns-server (`dns-server/`)
- **cmd/** - Application entrypoints (dns-server, agent-update)
- **internal/config/** - YAML configuration, dynamic config with listeners
- **internal/dnssvc/** - DNS resolution with middleware chain
  - **handlers/** - IP routing, static hosts handlers
  - **resolvers/** - DoH, DNS, mDNS, multi-provider resolvers
  - **middleware/** - Caching, TTL overrides, error handling
- **internal/routing/** - Route reconciliation with Keenetic router
- **internal/server/** - HTTP and UDP DNS server implementations
- **internal/cache/** - In-memory DNS cache with TTL-based expiration
- **internal/storage/** - Persistent DNS record storage
- **internal/web/** - Web UI with WebSocket streaming

### agent (`agent/`)
Lightweight service running on Keenetic router providing gRPC API:
- `ip route` management (add/delete)
- `ip rule` management (policy routing rules)
- Device/host listing via `ndmc` CLI

### Shared Packages (`internal/`)
- **log/** - Structured logging with buffering and prefixes
- **setup/** - Signal handling, pprof setup
- **stream/** - Buffered stream with cursor-based queries and listeners
- **tsv/** - TSV file parsing utilities
- **util/** - Set implementations, ring buffer, lazy map

## Configuration

YAML-based configuration in `config.yaml`:
- DNS upstream providers (DoH endpoints, DNS servers)
- Selective routing rules (domain patterns → interface)
- Static IP routes
- mDNS service definitions
- Reconciliation settings

## Key Patterns

### Dynamic Config with Listeners
`config.Dynamic[T]` provides thread-safe config with listener callbacks. Listeners must not call `Set()` synchronously (causes deadlock).

### Stream with Listeners
`stream.Buffered[T]` provides cursor-based queries and push updates via listeners. Remember to check context cancellation before adding listeners.

### IP Routing Flow
1. DNS query received
2. Response processed by `ipRoutingHandler`
3. A records extracted for configured domains
4. Routes added via agent RPC
5. Reconciliation loop syncs routes with router state

## Development Notes

### Running Tests
```bash
go test ./...
```

### Building
```bash
make lint build
```

### Code Style
- Standard Go formatting
- `slog` for structured logging
- iter.Seq2 for two-value iterators (Go 1.22+)

### Recent Changes
- Refactored DNS handlers and resolvers
- Improved route reconcile logging with action reason
- Added TSV column parsing utility
- Changed CPU architecture handling

## Known Issues (Documented)

None currently.


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
