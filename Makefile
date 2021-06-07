build:
	go build -o liquid-loop cmd/liquid-loop/main.go
	chmod a+x liquid-loop
copy:
	cp liquid-loop /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/alice/lightningd/plugins/
	cp liquid-loop /mnt/c/Users/kon-dev/.polar/networks/9/volumes/c-lightning/bob/lightningd/plugins/