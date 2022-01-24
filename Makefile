OUTDIR=./out

build:
	go build -tags dev -o $(OUTDIR)/peerswap ./cmd/peerswap/main.go
	chmod a+x $(OUTDIR)/peerswap
	go build -o $(OUTDIR)/peerswapd ./cmd/peerswaplnd/peerswapd/main.go
	chmod a+x $(OUTDIR)/peerswapd
	go build -o $(OUTDIR)/pscli ./cmd/peerswaplnd/pscli/main.go
	chmod a+x $(OUTDIR)/pscli
.PHONY: build

build-with-fast-test:
	go build -tags dev -tags fast_test -o $(OUTDIR)/peerswap ./cmd/peerswap/main.go
	chmod a+x $(OUTDIR)/peerswap
	go build -tags dev -tags fast_test -o $(OUTDIR)/peerswapd ./cmd/peerswaplnd/peerswapd/main.go
	chmod a+x $(OUTDIR)/peerswapd
.PHONY: build-with-fast-test

test: build-with-fast-test
	PAYMENT_RETRY_TIME=20 go test -tags dev -tags fast_test -timeout=10m -v ./...
.PHONY: test

test-integration: build-with-fast-test
	RUN_INTEGRATION_TESTS=1 PAYMENT_RETRY_TIME=20 LIGHTNING_TESTFRAMEWORK_FILTER="peerswap" go test -tags dev -tags fast_test -timeout=60m ./test
.PHONY: test-integration

lnd-release:
	go build -o peerswapd ./cmd/peerswaplnd/peerswapd/main.go
	go build -o pscli ./cmd/peerswaplnd/pscli/main.go
.PHONY: lnd-release

cln-release:
	go build -o peerswap ./cmd/peerswap/main.go
.PHONY: cln-release

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
    	--go-grpc_out=. --go-grpc_opt=paths=source_relative \
    	./peerswaprpc/peerswaprpc.proto

parse-states-md:
	go run ./contrib/stateparser.go --dir=./docs/mmd

IMG_WIDTH=1600
IMG_HEIGHT=400

docs/img/swap-in-receiver-states.png:
	sed 's/`//g' docs/mmd/swap-in-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-receiver-states.png
.PHONY: docs/img/swap-in-receiver-states.png

docs/img/swap-in-sender-states.png:
	sed 's/`//g' docs/mmd/swap-in-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-sender-states.png
.PHONY: docs/img/swap-in-sender-states.png

docs/img/swap-out-receiver-states.png:
	sed 's/`//g' docs/mmd/swap-out-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-receiver-states.png
.PHONY: docs/img/swap-out-receiver-states.png

docs/img/swap-out-sender-states.png:
	sed 's/`//g' docs/mmd/swap-out-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-sender-states.png
.PHONY: docs/img/swap-out-sender-states.png

docs/img/swap-in-sequence.png:
	sed 's/`//g' docs/mmd/swap-in-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-in-sequence.png
.PHONY: docs/img/swap-in-sequence.png

docs/img/swap-out-sequence.png:
	sed 's/`//g' docs/mmd/swap-out-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-out-sequence.png
.PHONY: docs/img/swap-out-sequence.png

docs: parse-states-md docs/img/swap-in-receiver-states.png docs/img/swap-in-sender-states.png docs/img/swap-out-receiver-states.png docs/img/swap-out-sender-states.png docs/img/swap-in-sequence.png docs/img/swap-out-sequence.png
.PHONY: docs

clean-docs:
	rm -f docs/mmd/swap-in-receiver-states.md
	rm -f docs/mmd/swap-out-receiver-states.md
	rm -f docs/mmd/swap-in-sender-states.md
	rm -f docs/mmd/swap-out-sender-states.md
	rm -f docs/img/swap-in-receiver-states.png
	rm -f docs/img/swap-in-sender-states.png
	rm -f docs/img/swap-out-sender-states.png
	rm -f docs/img/swap-out-receiver-states.png
	rm -f docs/img/swap-in-sequence.png
	rm -f docs/img/swap-out-sequence.png
.PHONY: clean-docs