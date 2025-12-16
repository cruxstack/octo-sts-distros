.PHONY: build test-unit lint vet fmt

# Build all cmd binaries
build:
	cd cmd && go build ./...

# Run unit tests across all modules
test-unit:
	cd internal && go test ./...
	cd cmd && go test ./...

# Run go vet across all modules
vet:
	cd internal && go vet ./...
	cd cmd && go vet ./...

# Check formatting
fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Lint (combines vet + fmt check)
lint: vet fmt
