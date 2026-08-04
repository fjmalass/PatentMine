# Client/Server RPC and HTTP Architecture

PatentMine is a daemon-centered application. The daemon owns the database,
filesystem access, runtime configuration, secrets, and business logic. User
interfaces are thin clients that ask the daemon to do work.

This boundary matters when adding features: the source of truth belongs in the
daemon/engine and is exposed through the RPC contract. TUI and HTTP code should
present or adapt that contract, not duplicate the underlying logic.

## Process Model

- `patentmine serve` starts the daemon.
- The daemon builds the `engine.Engine`, opens SQLite, resolves config, and serves
  local RPC calls on the configured Unix socket.
- `patentmine tui` is a terminal client. It connects to the daemon over RPC.
- CLI helper commands that need daemon state also connect over RPC.
- `patentmine api` starts an HTTP frontend. It connects to the same daemon over
  RPC and translates HTTP requests into daemon calls.
- Browser or remote clients talk to `patentmine api` over HTTP, not directly to
  the SQLite database or engine internals.

```text
TUI -------- RPC --------> daemon/engine
CLI -------- RPC --------> daemon/engine
Browser ---- HTTP API ---> api frontend ---- RPC ----> daemon/engine
```

## RPC Is The Internal Client/Server Boundary

RPC is PatentMine's internal client/server protocol. It is a JSON-RPC-style wire
contract carried over the daemon's Unix domain socket.

- Method names are typed constants in `internal/proto/proto.go`.
- Request and response payloads live in `internal/proto/*.go`.
- The daemon dispatch table is built in `internal/rpc/server.go`.
- Domain-specific RPC handlers live in `internal/rpc/server_*.go`.
- Clients use `internal/rpc.Client.Call` to invoke one daemon method and decode
  the typed result.

RPC methods should be stable operation names, for example:

- `patent.get`
- `patent.list`
- `deadline.list`
- `renewal.validation.list`
- `source.bibs.list`

When a feature needs daemon-owned data or behavior, add or reuse an RPC method
instead of reading files or database tables from a frontend.

## HTTP Is A Thin Web Adapter

HTTP is the browser/external integration surface. It is not a separate business
logic layer.

The HTTP server in `internal/api` should:

- Parse HTTP path, query, and body inputs.
- Build the matching `proto` params.
- Call the daemon through `internal/rpc.Client`.
- Map daemon results and errors to HTTP responses.

It should not:

- Open SQLite directly.
- Reimplement engine decisions.
- Read secret files directly.
- Read application-owned filesystem content that should be resolved by the
  daemon.

For HTTP route details, see [REST API](./05_REST_API.md).

## Layer Responsibilities

| Layer | Owns | Must not own |
| --- | --- | --- |
| `engine` | Business logic, database-backed decisions, filesystem operations, source-of-truth rules | UI presentation |
| `proto` | Shared method names and typed request/response payloads | Implementation logic |
| `rpc` server | Decode RPC params, call engine, return proto results | Business rules outside engine |
| `tui` | Keyboard/UI state, overlays, panes, rendering daemon results | Database/filesystem source-of-truth logic |
| `api` | HTTP routing, auth/CORS/TLS wrapping, HTTP-to-RPC adaptation | Independent business logic |
| Web/GUI clients | Presentation and interaction | Direct database, secret, or daemon filesystem access |

## Feature Placement Rule

For any client-visible feature:

1. Put source-of-truth behavior in `internal/engine` or a daemon-owned service
   called by the engine.
2. Define the wire contract in `internal/proto`.
3. Expose the operation through `internal/rpc`.
4. Add TUI presentation only if terminal users need it.
5. Add HTTP routes only if browser, GUI, script, or remote users need it.
6. Keep frontend behavior thin: frontend code should format inputs, call RPC, and
   render outputs.

This keeps the daemon authoritative and prevents different frontends from slowly
implementing different versions of the same operation.

## Documentation Browser Example

The documentation browser should follow the same architecture.

Correct placement:

- The daemon owns the configured documentation directory.
- The daemon docs index should also include root-level project docs such as
  `README.md` and `CHANGELOG.md`, so TUI/API clients can read them even though
  they are not numbered files under `docs/`.
- The daemon lists available docs and reads document content.
- The daemon validates relative document IDs and rejects path traversal.
- RPC exposes methods such as `docs.list` and `docs.get`.
- The TUI calls those RPC methods and displays the returned list/content.
- HTTP routes, if exposed, forward to the same RPC methods.

Incorrect placement:

- The TUI scans `docs/` directly.
- The TUI reads Markdown files directly from disk.
- The HTTP API reads a separate docs directory with different rules.
- The GUI invents a separate document ID scheme from the daemon.

Future Markdown preview or HTML generation should stay behind the same boundary:
the daemon/API contract should decide the modes and outputs, while clients render
or display what they receive.

## Adding A New RPC-Backed Feature

Use this checklist when adding daemon-backed behavior:

1. Add or update the engine method.
2. Add `proto.Method...` constants and typed params/results.
3. Add an RPC handler in the relevant `internal/rpc/server_*.go` file.
4. Register the handler in `internal/rpc/server.go`.
5. Add command catalog entries when the action is user-invoked.
6. Add TUI panes/overlays that call RPC instead of reading local state directly.
7. Add HTTP handlers only when web or external clients need the feature.
8. Test engine behavior first, then RPC/API adapters where useful.

## Security Boundary

The daemon is also the right place for privileged configuration and secret-backed
operations.

- TUI and GUI clients should not read raw credential files.
- HTTP routes should expose redacted status or daemon-computed results, not secret
  material.
- Build/deploy secret handling is documented in
  [Build / Deploy Secret Architecture](./22_BUILD_DEPLOY_SECRETS.md).
