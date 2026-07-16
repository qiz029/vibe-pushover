.PHONY: build test vet

build:
	go build -o vibe-pushover ./cmd/vibe-pushover

test:
	go test ./...

vet:
	go vet ./...
