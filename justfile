set shell := ["bash", "-cu"]

gen:
    go generate ./...

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
	@echo "Done! Now run: git push origin main --tags"
