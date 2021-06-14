build:
	go build -o liquid-swap ./cmd/liquid-swap/.
	chmod a+x liquid-swap
copy:
	cp liquid-swap /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/alice/lightningd/plugins/
	cp liquid-swap /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/bob/lightningd/plugins/

