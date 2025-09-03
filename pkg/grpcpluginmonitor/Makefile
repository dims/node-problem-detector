# Makefile for GRPC Plugin Monitor

.PHONY: proto clean

# Generate protobuf files
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/plugin_monitor.proto

# Clean generated files
clean:
	rm -f proto/*.pb.go
	rm -f *.test
	rm -f */test
	rm -f */*.test

# Run tests
test:
	go test ./...

# Install protobuf tools
install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest