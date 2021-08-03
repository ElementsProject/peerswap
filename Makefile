build:
	go build -tags dev -o peerswap ./cmd/peerswap/main.go
	chmod a+x peerswap
test:
	go test -count=1 -v ./...
release:
	go build -o peerswap ./cmd/peerswap/main.go