.PHONY: build test run clean tidy help install

BINARY_NAME := orche

# Build binary
build:
	go build -o $(BINARY_NAME) ./cmd/orche

# Run tests
test:
	go test ./...

# Run the binary
run: build
	./$(BINARY_NAME)

# Run the simple example
run-example:
	go run ./examples/simple/

# Install binary
install: build
	@if [ -w /usr/local/bin ]; then \
		cp $(BINARY_NAME) /usr/local/bin/; \
	else \
		mkdir -p $$HOME/.local/bin; \
		cp $(BINARY_NAME) $$HOME/.local/bin/; \
		echo "Installed to $$HOME/.local/bin (ensure it's in PATH)"; \
	fi

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME) *.test

# Tidy go modules
tidy:
	go mod tidy

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build binary"
	@echo "  test          - Run tests"
	@echo "  run           - Build and run binary"
	@echo "  run-example   - Run example"
	@echo "  install       - Install binary"
	@echo "  clean         - Clean build artifacts"
	@echo "  tidy          - Tidy go modules"
