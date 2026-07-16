.PHONY: build release test vet

build:
	go build -o vibe-pushover ./cmd/vibe-pushover

release:
	@test -n "$(VERSION)" || (echo "VERSION is required, for example: make release VERSION=v0.1.0" >&2; exit 1)
	VERSION="$(VERSION)" scripts/build-release.sh

test:
	go test ./...

vet:
	go vet ./...
