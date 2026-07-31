---
inclusion: auto
---

# Build, Test, and Run Reference

## Prerequisites

| Tool | Minimum Version | Install |
|------|----------------|---------|
| Go | 1.23 | https://go.dev/dl |
| Rust + Cargo | 1.85 | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| C compiler | any | macOS: `xcode-select --install` / Linux: `apt install gcc` |

Verify:
```bash
go version        # must be 1.23+
rustc --version   # must be 1.85+
cargo --version   # must be 1.85+
```

---

## Build

### Full build (recommended)

```bash
make build
```

This runs both steps: Rust static library → Go binary.

### Manual steps

```bash
# Step 1: Build Rust static library
cd rust-core && cargo build --release && cd ..
# Produces: rust-core/target/release/librust_core.a

# Step 2: Build Go binary (CGO is required)
CGO_ENABLED=1 go build -o bin/hybrid-runtime-blockchain-engine ./cmd/server
```

> **CGO_ENABLED=1 is always required.** Never build with `CGO_ENABLED=0`.

---

## Run

### With a real Ethereum node

```bash
export ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-API-KEY"
./bin/hybrid-runtime-blockchain-engine
```

### Without a real node (load test / development mode)

```bash
ETH_RPC_URL=ws://localhost:8545 LOAD_TEST_ENABLED=true \
  ./bin/hybrid-runtime-blockchain-engine
```

The block streamer will fail to connect (expected in this mode) and log a warning. The rest of the system starts normally and you can drive it via MCP load test calls.

### Using make

```bash
ETH_RPC_URL="wss://..." make run
```

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ETH_RPC_URL` | **Yes** | — | Ethereum WebSocket RPC endpoint |
| `WORKER_COUNT` | No | `4` | Worker goroutines (1–256) |
| `METRICS_PORT` | No | `9090` | Prometheus + health HTTP port |
| `MCP_PORT` | No | `8080` | MCP JSON-RPC port |
| `LOAD_TEST_ENABLED` | No | `false` | Skip real Ethereum connection |

⚠️ **Known bug**: The `ETH_RPC_URL` secret validator incorrectly rejects valid Infura/Alchemy URLs because their API keys are 32-char hex strings in the URL path. Workaround: this bug is tracked in CLAUDE.md technical debt.

---

## Test

### All tests (recommended)

```bash
make test
# Equivalent to:
CGO_ENABLED=1 go test ./... -v -race -timeout=120s
cd rust-core && cargo test
```

### Go tests only — packages that don't need cgo

```bash
CGO_ENABLED=0 go test \
  ./internal/config/... \
  ./internal/logging/... \
  ./internal/shutdown/... \
  ./internal/loadtest/... \
  ./internal/mcp/... \
  ./internal/worker/... \
  -v -race
```

### Go tests only — packages that need the Rust library

```bash
CGO_ENABLED=1 go test \
  ./internal/ffi/... \
  ./internal/reorg/... \
  ./internal/metrics/... \
  ./internal/streamer/... \
  -v -race
```

### Rust tests only

```bash
cd rust-core && cargo test
```

### With coverage report

```bash
CGO_ENABLED=1 go test ./... -coverprofile=coverage.out -timeout=120s
go tool cover -func=coverage.out | tail -1          # total
go tool cover -html=coverage.out -o coverage.html   # visual
open coverage.html
```

Or: `make coverage` (builds the HTML report automatically)

### Current coverage by package

| Package | Coverage |
|---------|----------|
| shutdown | 100% |
| mcp | 94% |
| config | 93% |
| loadtest | 92% |
| logging | 90% |
| worker | 89% |
| reorg | 86% |
| ffi | 83% |
| streamer | 82% |
| metrics | 80% |

---

## Benchmarks

```bash
make bench

# Or manually:
CGO_ENABLED=1 go test ./... -bench=. -benchmem -run=^$ -timeout=120s
cd rust-core && cargo bench
```

Key benchmarks to watch:
- `BenchmarkApplyBlock` — Rust FFI round-trip latency
- `BenchmarkSerializeBlock` — Go binary serialization overhead
- `BenchmarkReorgRollback` — State rollback latency

---

## Makefile Targets Reference

| Target | What it does |
|--------|-------------|
| `make build` | Rust library + Go binary |
| `make run` | Build and run (requires `ETH_RPC_URL`) |
| `make test` | Rust tests + Go tests with race detector |
| `make bench` | Rust + Go benchmarks |
| `make coverage` | Open HTML coverage report |
| `make docker` | Build Docker image |
| `make clean` | Remove `bin/`, `rust-core/target/`, `coverage.*` |

---

## Docker

### Build image

```bash
docker build -t hybrid-runtime-blockchain-engine:latest -f docker/Dockerfile .
```

The Dockerfile uses a three-stage build:
1. **rust-builder**: compiles `rust-core` → `librust_core.a`
2. **go-builder**: compiles Go binary with cgo linking against the Rust library
3. **runtime**: debian-slim with only the binary + runtime libs

### Run with Docker

```bash
docker run -d \
  --name blockchain-engine \
  -e ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-KEY" \
  -e WORKER_COUNT=4 \
  -p 9090:9090 \
  -p 8080:8080 \
  hybrid-runtime-blockchain-engine:latest
```

### Run with Docker Compose (includes Prometheus)

```bash
ETH_RPC_URL="wss://..." docker-compose up
```

Starts the engine (ports 9090, 8080) plus Prometheus (port 9091).

---

## Verify a Running System

### Health check

```bash
curl http://localhost:9090/health
# Returns: "OK" (200) or error message (503)
```

> In load-test mode, `/health` returns 503 (streamer not connected). Use `/livez` instead.

```bash
curl http://localhost:9090/livez   # always 200 if process is running
curl http://localhost:9090/readyz  # 200 when ready to serve
```

### Prometheus metrics

```bash
curl -s http://localhost:9090/metrics | grep -E "^(worker|rust|reorg|blocks|goroutine)"
```

### State debug

```bash
curl -s http://localhost:9090/debug/state | python3 -m json.tool
```

### MCP tool call

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_state_root","params":{},"id":1}'
```

---

## CI Pipeline

The GitHub Actions CI (`.github/workflows/ci.yml`) runs:

1. `cargo test` — Rust unit tests
2. `go test ./... -race -coverprofile=coverage.out` — Go tests with race detector
3. `go build ./cmd/server` — verify binary compiles
4. `docker build` — verify Docker image builds

All must pass before a PR is merged.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `library not found: -lrust_core` | Rust not built | Run `cd rust-core && cargo build --release` |
| `could not load DWARF` / cgo errors | Old build artifacts | `make clean && make build` |
| `CGO_ENABLED` warnings | Building without cgo | Always use `CGO_ENABLED=1` |
| `Failed to start Block Streamer` exits | No Ethereum node | Set `LOAD_TEST_ENABLED=true` |
| `ETH_RPC_URL` rejected at startup | Secret detector bug | See tech debt in CLAUDE.md |
| HTTP 429 from MCP server | Rate limit (10 req/min) | Wait 60s, or restart |
| `cargo` not found | Rust not on PATH | `echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc` |
| Race condition in tests | Concurrency bug | Run with `-race`, check for missing mutex/channel |
