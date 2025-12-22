.PHONY: run test build css css-watch lint clean dev

# Default port
PORT ?= 8080

# Run the server
run:
	go run ./cmd/server

# Run with live reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air

# Run all tests
test:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Build the binary
build:
	go build -o bin/server ./cmd/server

# Build Tailwind CSS
css:
	npx tailwindcss -i web/static/css/input.css -o web/static/css/app.css --minify

# Watch Tailwind CSS for changes
css-watch:
	npx tailwindcss -i web/static/css/input.css -o web/static/css/app.css --watch

# Run linters
lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f data/wealth.db

# Initialize database (run migrations)
db-init:
	go run ./cmd/server -migrate

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Check for security issues (requires gosec)
security:
	@which gosec > /dev/null 2>&1 || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec ./...

# Install development dependencies
deps:
	go mod tidy
	npm install

# All pre-commit checks
check: fmt lint test
