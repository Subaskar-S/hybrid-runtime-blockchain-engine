.PHONY: build run test bench clean coverage docker

# ── Build ─────────────────────────────────────────────────────────────────────
build: build-rust build-go

build-rust:
	cd rust-core && cargo build --release

build-go:
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(PWD)/rust-core/target/release" \
	go build -o bin/hybrid-runtime-blockchain-engine ./cmd/server

# ── Run ───────────────────────────────────────────────────────────────────────
run: build
	ETH_RPC_URL=$${ETH_RPC_URL:-ws://localhost:8545} \
	WORKER_COUNT=$${WORKER_COUNT:-4} \
	METRICS_PORT=$${METRICS_PORT:-9090} \
	MCP_PORT=$${MCP_PORT:-8080} \
	LOAD_TEST_ENABLED=$${LOAD_TEST_ENABLED:-false} \
	./bin/hybrid-runtime-blockchain-engine

# ── Test ──────────────────────────────────────────────────────────────────────
test: test-rust test-go

test-rust:
	cd rust-core && cargo test

test-go:
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(PWD)/rust-core/target/release" \
	go test ./... -v -race -coverprofile=coverage.out

# ── Benchmarks ────────────────────────────────────────────────────────────────
bench: bench-rust bench-go

bench-rust:
	cd rust-core && cargo bench

bench-go:
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(PWD)/rust-core/target/release" \
	go test ./... -bench=. -benchmem -run=^$

# ── Coverage ──────────────────────────────────────────────────────────────────
coverage:
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

# ── Docker ────────────────────────────────────────────────────────────────────
docker:
	docker build -t hybrid-runtime-blockchain-engine:latest -f docker/Dockerfile .

# ── Clean ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf rust-core/target
	rm -f bin/hybrid-runtime-blockchain-engine
	rm -f coverage.out coverage.html
	rm -f *.test
	rm -f benchmark_report_*.json
