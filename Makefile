.PHONY: build test release pytest

build:
	go build -tags dev -o peerswap ./cmd/peerswap/main.go
	chmod a+x peerswap

test:
	go test -count=1 -v ./...

release:
	go build -o peerswap ./cmd/peerswap/main.go

pytest: build
	pytest ./peerswap_pytest
