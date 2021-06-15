build:
	go build -o liquid-swap-plugin ./cmd/liquid-swap/.
	chmod a+x liquid-swap-plugin
test:
	go test -count=1 -v ./...