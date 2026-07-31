.PHONY: test verify build release

test:
	go test ./...

verify:
	go vet ./...
	go test -race ./...

build:
	mkdir -p output
	CGO_ENABLED=0 go build -trimpath -o output/agent-telemetry ./cmd/agent-telemetry

release:
	./scripts/build-release.sh
