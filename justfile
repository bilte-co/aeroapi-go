set shell := ["bash", "-cu"]

gen:
    go generate ./...

format:
    go run cmd/specfmt/main.go format api.yaml -o formatted.yaml -v

test:
	@echo "Running tests..."
	go test -v ./...

modernize:
	@echo "Running gopls modernize..."
	go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix -test ./...

lint:
	@echo "Running golangci-lint..."
	golangci-lint run ./...

sec:
	@echo "Running gosec..."
	gosec ./...

vuln:
	@echo "Running govulncheck..."
	govulncheck ./...

check:
	@echo "Running full checks (test, lint, vuln)..."
	go test ./...
	golangci-lint run ./...
	govulncheck ./...

# Usage: just release 0.1.0
release version:
	@echo "Preparing release v{{version}}..."
	git diff --quiet || (echo "Working tree not clean"; exit 1)
	just check
	@echo "Tagging v{{version}}..."
	git tag "v{{version}}"
	@echo "Done! Now run: git push origin main --tags && just publish {{version}}"

# Trigger Go module proxy indexing after pushing a tag
publish version:
	@echo "Publishing v{{version}} to Go module proxy..."
	GOPROXY=proxy.golang.org go list -m github.com/bilte-co/aeroapi-go@v{{version}}
