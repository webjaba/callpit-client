.PHONY: run build test test-race vet

run:
	go run ./cmd/calls

build:
	go build ./cmd/calls

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...
