build:
	go build -o liquid-swap-plugin ./cmd/liquid-swap/.
	chmod a+x liquid-swap-plugin
copy:
	cp liquid-swap-plugin /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/alice/lightningd/plugins/
	cp liquid-swap-plugin /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/bob/lightningd/plugins/

test-ci:
	export API_URL=http://localhost:3001; \
	export API_BTC_URL=http://localhost:3000; \
	go test -count=1 -v ./..
test:
	go test -count=1 -v ./...