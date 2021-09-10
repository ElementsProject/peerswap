.PHONY: build test release pytest

build:
	go build -tags dev -o peerswap ./cmd/peerswap/main.go
	chmod a+x peerswap

test: build
	go test -race -count=1 -v ./...

test_all: build
	go test -count=1 --tags docker ./...

release:
	go build -o peerswap ./cmd/peerswap/main.go

pytest: build
	pytest ./test
