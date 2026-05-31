# ── Stage 1: Build Rust static library ──────────────────────────────────────
FROM rust:1.75-slim AS rust-builder

WORKDIR /build/rust-core
COPY rust-core/ .
RUN cargo build --release

# ── Stage 2: Build Go binary ─────────────────────────────────────────────────
FROM golang:1.23-bookworm AS go-builder

WORKDIR /build

# Copy Rust artefacts
COPY --from=rust-builder /build/rust-core/target/release/librust_core.a \
     /build/rust-core/target/release/librust_core.a

# Download Go dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with cgo enabled, linking the Rust static library
RUN CGO_ENABLED=1 \
    CGO_LDFLAGS="-L/build/rust-core/target/release" \
    go build -o /build/bin/hybrid-runtime-blockchain-engine ./cmd/server

# ── Stage 3: Minimal runtime image ───────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /build/bin/hybrid-runtime-blockchain-engine /usr/local/bin/

# Prometheus metrics
EXPOSE 9090
# MCP server
EXPOSE 8080

ENV WORKER_COUNT=4 \
    METRICS_PORT=9090 \
    MCP_PORT=8080 \
    LOAD_TEST_ENABLED=false

# Health check using the liveness probe endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:9090/livez || exit 1

ENTRYPOINT ["/usr/local/bin/hybrid-runtime-blockchain-engine"]
