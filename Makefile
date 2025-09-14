.PHONY: run run-clean tidy build test

run:
	go run ./cmd/bot

run-clean:
	@echo "[run-clean] Purging Go caches and toolchain artifacts..."
	go clean -cache -testcache -modcache
	- rm -rf "$(shell go env GOCACHE)"
	- rm -rf "$(shell go env GOPATH)/pkg/mod/golang.org/toolchain"*
	@echo "[run-clean] Toolchain/version info:"
	GOTOOLCHAIN=local go version || true
	GOTOOLCHAIN=local go env GOVERSION GOROOT || true
	@echo "[run-clean] Running bot with local toolchain..."
	GOTOOLCHAIN=local go run ./cmd/bot

tidy:
	go mod tidy

build:
	go build -o bin/bot ./cmd/bot

test:
	go clean -cache -testcache -modcache
	go test ./...
