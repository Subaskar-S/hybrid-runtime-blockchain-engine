.PHONY: build run test bench clean

# Build targets
build: build-rust build-go

build-rust:
	cd rust-core && cargo build --release

build-go:
	CGO_ENABLED=1 go build -o bin/hybrid-runtime-blockchain-engine ./cmd/server

# Run target
run: build
	./bin/hybrid-runtime-blockchain-engine

# Test targets
test: test-rust test-go

test-rust:
	cd rust-core && cargo test

test-go:
	go test ./... -v -race -coverprofile=coverage.out

# Benchmark targets
bench: bench-rust bench-go

bench-rust:
	cd rust-core && cargo bench

bench-go:
	go test ./... -bench=. -benchmem

# Coverage
coverage:
	go tool cover -html=coverage.out

# Clean target
clean:
	rm -rf rust-core/target
	rm -f bin/hybrid-runtime-blockchain-engine
	rm -f coverage.out
	rm -f *.test
