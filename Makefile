build:
	go build -o liquid-swap-plugin ./cmd/peerswap/.
	chmod a+x liquid-swap-plugin
test:
	go test -count=1 -v ./...
release:
	go build