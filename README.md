# PatentMine

PatentMine documentation now lives under [`docs/`](./docs/), numbered in reading order.

Start with [`docs/01_README.md`](./docs/01_README.md) for the architectural and lifecycle manual.

PatentMine is daemon-centered: `patentmine serve` owns the database, filesystem
access, secrets, and engine logic. Thin clients talk to it through RPC; the HTTP
API is a web-facing adapter that forwards requests to the same daemon RPC layer.

See [`docs/23_CLIENT_SERVER_RPC_ARCHITECTURE.md`](./docs/23_CLIENT_SERVER_RPC_ARCHITECTURE.md)
for the RPC/HTTP client-server model.
