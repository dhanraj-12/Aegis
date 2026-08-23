# Variables
BINARY_NAME=aegis
PROXY_CMD=cmd/proxy/main.go

.PHONY: all deps proto build run clean

# The default target if you just type 'make'
all: run

# 1. Install dependencies and the Go protoc plugin
# 1. Install dependencies and the Go protoc plugin
deps:
	go mod tidy
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.33.0

# 2. Generate the Protobuf Go structs
proto:
	@echo "Generating Protobuf files..."
	protoc --go_out=. --go_opt=paths=source_relative api/proto/state.proto

# 3. Build the Go binary (Depends on 'proto' running first)
build: proto
	@echo "Compiling the Aegis binary..."
	go build -o bin/$(BINARY_NAME) $(PROXY_CMD)

# 4. Run the proxy (Depends on 'build' running first)
run: build
	@echo "Starting Aegis..."
	./bin/$(BINARY_NAME)

# 5. Clean up generated files and binaries
clean:
	@echo "Cleaning up workspace..."
	rm -rf bin/
	rm -f api/proto/*.pb.go