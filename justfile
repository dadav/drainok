# drainok development tasks

# List available recipes
default:
    @just --list

# Build the binary into ./drainok
build:
    go build -o drainok .

# Run all tests
test:
    go test ./...

# Run go vet, plus golangci-lint if installed
lint:
    go vet ./...
    @if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi

# Format all Go files
fmt:
    gofmt -w .

# Run the tool against the current kube context
run *ARGS:
    go run . {{ ARGS }}

# Build a local snapshot release with goreleaser (no publishing)
snapshot:
    goreleaser release --snapshot --clean

# Validate the goreleaser configuration
goreleaser-check:
    goreleaser check

# Create the local kind test cluster (1 control-plane, 2 workers)
kind-up:
    kind create cluster --name drainok --config kind-config.yaml

# Delete the local kind test cluster
kind-down:
    kind delete cluster --name drainok

# Update all dependencies
update:
    go get -u -t ./...
    go mod tidy
