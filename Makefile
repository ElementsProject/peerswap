OUTDIR=./out
PAYMENT_RETRY_TIME=10
PEERSWAP_TEST_FILTER="peerswap"
GIT_COMMIT=$(shell git rev-list -1 HEAD)

INTEGRATION_TEST_ENV= \
	RUN_INTEGRATION_TESTS=1 \
	PAYMENT_RETRY_TIME=$(PAYMENT_RETRY_TIME) \
	PEERSWAP_TEST_FILTER=$(PEERSWAP_TEST_FILTER)

INTEGRATION_TEST_OPTS= \
	-tags dev \
	-tags fast_test \
	-timeout=30m -v

.DEFAULT_GOAL := release

release: lnd-release cln-release
.PHONY: release

build:
	go build -tags dev -o $(OUTDIR)/peerswap-plugin -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswap-plugin/main.go
	chmod a+x $(OUTDIR)/peerswap-plugin
	go build -o $(OUTDIR)/peerswapd -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/peerswapd/main.go
	chmod a+x $(OUTDIR)/peerswapd
	go build -o $(OUTDIR)/pscli -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/pscli/main.go
	chmod a+x $(OUTDIR)/pscli
.PHONY: build


build-with-fast-test:
	go build -tags dev -tags fast_test -o $(OUTDIR)/peerswap-plugin ./cmd/peerswap-plugin/main.go
	chmod a+x $(OUTDIR)/peerswap-plugin
	go build -tags dev -tags fast_test -o $(OUTDIR)/peerswapd ./cmd/peerswaplnd/peerswapd/main.go
	chmod a+x $(OUTDIR)/peerswapd
.PHONY: build-with-fast-test

test: build-with-fast-test
	PAYMENT_RETRY_TIME=5 go test -tags dev -tags fast_test -timeout=10m -v ./...
.PHONY: test

test-integration: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} ./test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} ./lnd
.PHONY: test-integration

test-bitcoin-cln: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_ClnCln_Bitcoin_SwapOut|'\
	'Test_ClnCln_Bitcoin_SwapIn|'\
	'Test_ClnLnd_Bitcoin_SwapOut|'\
	'Test_ClnLnd_Bitcoin_SwapIn)'\
	 ./test
.PHONY: test-bitcoin-cln

test-bitcoin-lnd: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_LndLnd_Bitcoin_SwapOut|'\
	'Test_LndLnd_Bitcoin_SwapIn|'\
	'Test_LndCln_Bitcoin_SwapOut|'\
	'Test_LndCln_Bitcoin_SwapIn)'\
	 ./test
.PHONY: test-liquid-lnd

test-liquid-cln: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_ClnCln_Liquid_SwapOut|'\
	'Test_ClnCln_Liquid_SwapIn|'\
	'Test_ClnLnd_Liquid_SwapOut|'\
	'Test_ClnLnd_Liquid_SwapIn)'\
	 ./test
.PHONY: test-liquid-cln

test-liquid-lnd: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_LndLnd_Liquid_SwapOut|'\
	'Test_LndLnd_Liquid_SwapIn|'\
	'Test_LndCln_Liquid_SwapOut|'\
	'Test_LndCln_Liquid_SwapIn)'\
	 ./test
.PHONY: test-liquid-lnd

test-misc-integration: build-with-fast-test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_GrpcReconnectStream|'\
	'Test_GrpcRetryRequest|'\
	'Test_ClnCln_MPP|'
	'Test_ClnLnd_MPP)'\
	 ./test
.PHONY: test-misc-integration

lnd-release: clean-lnd
	go install -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/peerswapd
	go install -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/pscli
.PHONY: lnd-release

<<<<<<< HEAD
cln-release: clean-cln
	# peerswap-plugin binary is not installed in GOPATH because it must be called by full pathname as a CLN plugin.
	# You may choose to install it to any location you wish.
=======
cln-release: 
>>>>>>> aeb0a84 (make: Add lnd package to the integration test command.)
	go build -o peerswap-plugin -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswap-plugin/main.go
.PHONY: cln-release

clean-cln:
	# PeerSwap CLN builds
	rm -f peerswap-plugin
	rm -f out/peerswap-plugin
.PHONY: clean-cln

clean-lnd:
	# PeerSwap LND builds
	rm -f out/peerswapd
	rm -f out/pscli
.PHONY: clean-lnd

clean: clean-cln clean-lnd
.PHONY: clean

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
    	--go-grpc_out=. --go-grpc_opt=paths=source_relative \
    	./peerswaprpc/peerswaprpc.proto
	protoc --grpc-gateway_out . \
        --grpc-gateway_opt logtostderr=true \
        --grpc-gateway_opt paths=source_relative \
        --grpc-gateway_opt grpc_api_configuration=./peerswaprpc/peerswap.yaml \
		./peerswaprpc/peerswaprpc.proto
	protoc -I . --openapiv2_out . \
        --openapiv2_opt logtostderr=true \
        --openapiv2_opt grpc_api_configuration=./peerswaprpc/peerswap.yaml \
        ./peerswaprpc/peerswaprpc.proto
.PHONY: proto

docs/mmd/swap-in-receiver-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-in-receiver-states.md -fsm swap_in_receiver

docs/mmd/swap-in-sender-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-in-sender-states.md -fsm swap_in_sender

docs/mmd/swap-out-receiver-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-out-receiver-states.md -fsm swap_out_receiver

docs/mmd/swap-out-sender-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-out-sender-states.md -fsm swap_out_sender

IMG_WIDTH=1600
IMG_HEIGHT=400

docs/img/swap-in-receiver-states.png: docs/mmd/swap-in-receiver-states.md
	sed 's/`//g' docs/mmd/swap-in-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-receiver-states.png
.PHONY: docs/img/swap-in-receiver-states.png

docs/img/swap-in-sender-states.png: docs/mmd/swap-in-sender-states.md
	sed 's/`//g' docs/mmd/swap-in-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-sender-states.png
.PHONY: docs/img/swap-in-sender-states.png

docs/img/swap-out-receiver-states.png: docs/mmd/swap-out-receiver-states.md
	sed 's/`//g' docs/mmd/swap-out-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-receiver-states.png
.PHONY: docs/img/swap-out-receiver-states.png

docs/img/swap-out-sender-states.png: docs/mmd/swap-out-sender-states.md
	sed 's/`//g' docs/mmd/swap-out-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-sender-states.png
.PHONY: docs/img/swap-out-sender-states.png

docs/img/swap-in-sequence.png: docs/mmd/swap-in-sequence.md
	sed 's/`//g' docs/mmd/swap-in-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-in-sequence.png
.PHONY: docs/img/swap-in-sequence.png

docs/img/swap-out-sequence.png: docs/mmd/swap-out-sequence.md
	sed 's/`//g' docs/mmd/swap-out-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-out-sequence.png
.PHONY: docs/img/swap-out-sequence.png

docs: docs/img/swap-in-receiver-states.png docs/img/swap-in-sender-states.png docs/img/swap-out-receiver-states.png docs/img/swap-out-sender-states.png docs/img/swap-in-sequence.png docs/img/swap-out-sequence.png
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
