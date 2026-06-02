# PatentMine Web UI

Browser client for the PatentMine REST API (TUI parity target).

## Development

```bash
# Terminal 1
patentmine serve

# Terminal 2
patentmine api --addr 127.0.0.1:18080

# Terminal 3
cd web && npm install && npm run dev
```

Open the Vite dev server; API calls are proxied to port 18080.

## Production

```bash
cd web && npm run build
patentmine api --web-dir ./web/dist
```

Browse to `http://127.0.0.1:18080/ui/`.

Remote access: see [REMOTE_API.md](../REMOTE_API.md).
