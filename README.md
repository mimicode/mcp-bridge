# mcp_bridge

Convert local stdio MCP servers into Streamable HTTP endpoints with one shared HTTP port and multiple routes.

## Features

- Reads stdio MCP server definitions from a JSON config file
- Reuses long-lived stdio child processes instead of spawning on every request
- Maps each enabled backend to its own Streamable HTTP route
- Mirrors backend tools, resources, prompts, and completions when supported
- Refreshes mirrored descriptors when backend list-changed notifications arrive
- Retries once after recoverable transport failures by restarting the backend

## Run

```bash
go run ./cmd/mcp-bridge -config ./config.json -listen :8080
```

If a server entry does not specify `path`, the program generates one under `/mcp` by default:

- `filesystem` -> `/mcp/filesystem`
- `mcp-server-fetch` -> `/mcp/mcp-server-fetch`

## Config

Use `config.example.json` as a template. The expected format is:

```json
{
  "mcpServers": {
    "name": {
      "enabled": true,
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "env": {
        "PYTHONIOENCODING": "utf-8"
      },
      "description": "optional description",
      "timeout": 60000,
      "path": "/mcp/custom-name"
    }
  }
}
```

## Endpoints

- `/` returns route metadata
- `/healthz` returns route readiness
- `/_admin/` opens the embedded config page
- `/_admin/api/config` gets or saves config json
- `/mcp/<route>` is the Streamable HTTP MCP endpoint for a configured backend

## Admin UI

- The admin UI is now a standalone Vue 3 project under `web/`
- The UI uses `Naive UI` for table, modal and form components
- The MCP table supports search, status filter, per-row startup test and quick route copy
- Local dev helper:

```bash
./scripts/dev.sh
```

- Optional overrides by environment:

```bash
CONFIG_PATH=./config.example.json LISTEN_ADDR=:8080 BASE_PATH=/mcp ./scripts/dev.sh
```

- Optional overrides by flags:

```bash
./scripts/dev.sh --config ./config.example.json --listen :8080 --base-path /mcp
```

- Useful flags:

```bash
./scripts/dev.sh --skip-build
./scripts/dev.sh --skip-install
./scripts/dev.sh --help
```

- Build the UI with:

```bash
cd web
npm install
npm run build
```

- The build output is emitted into `internal/ui/dist` and embedded into the Go binary

## Package Script

- One-click package script: `scripts/package-all.sh`
- The script will:
  - run `npm ci && npm run build` under `web/`
  - optionally run `go test ./...`
  - cross-compile macOS, Linux and Windows binaries
  - bundle `README.md` and `config.example.json`
- Default targets:
  - `darwin/amd64`
  - `darwin/arm64`
  - `linux/amd64`
  - `linux/arm64`
  - `windows/amd64`
  - `windows/arm64`
- Example:

```bash
./scripts/package-all.sh
```

- Optional overrides:

```bash
VERSION=v0.1.0 TARGETS="darwin/arm64 linux/amd64 windows/amd64" ./scripts/package-all.sh
```

- The script injects build metadata into the binary:
  - `Version`
  - `Commit`
  - `BuildTime`
- Runtime check:

```bash
./mcp-bridge -version
```

## Hot Reload

- Saving from the admin page rewrites `config.json` and hot reloads only MCP routes
- Unchanged route definitions are reused
- Changed or removed route definitions are recreated or closed
- Listen address still comes from the process flag and requires restart if changed
