OUTDIR=./out
TEST_BIN_DIR=${OUTDIR}/test-builds
PAYMENT_RETRY_TIME=10
PEERSWAP_TEST_FILTER="peerswap"
GIT_COMMIT=$(shell git rev-list -1 HEAD)

BUILD_OPTS= \
	-ldflags "-X main.GitCommit=$(shell git rev-parse HEAD)"

TEST_BUILD_OPTS= \
	-ldflags "-X main.GitCommit=$(shell git rev-parse HEAD)" \
	-tags "dev fast_test"

INTEGRATION_TEST_ENV= \
	RUN_INTEGRATION_TESTS=1 \
	PAYMENT_RETRY_TIME=$(PAYMENT_RETRY_TIME) \
	PEERSWAP_TEST_FILTER=$(PEERSWAP_TEST_FILTER)

INTEGRATION_TEST_OPTS= \
	-tags dev \
	-tags fast_test \
	-timeout=30m -v \
	-failfast \
	-count=1

BINS= \
	${OUTDIR}/peerswapd \
	${OUTDIR}/pscli \
	${OUTDIR}/peerswap \

TEST_BINS= \
	${TEST_BIN_DIR}/peerswapd \
	${TEST_BIN_DIR}/pscli \
	${TEST_BIN_DIR}/peerswap \

.PHONY: subdirs ${BINS} ${TEST_BINS}

include peerswaprpc/Makefile
include docs/Makefile

release: lnd-release cln-release
.PHONY: release

clean-bins:
	rm ${BINS}

bins: ${BINS}

test-bins: ${TEST_BINS}

# Binaries for local testing and the integration tests.
${OUTDIR}/peerswapd:
	go build ${BUILD_OPTS} -o ${OUTDIR}/peerswapd ./cmd/peerswaplnd/peerswapd
	chmod a+x out/peerswapd

${OUTDIR}/pscli:
	go build ${BUILD_OPTS} -o ${OUTDIR}/pscli ./cmd/peerswaplnd/pscli
	chmod a+x out/pscli

${OUTDIR}/peerswap:
	go build ${BUILD_OPTS} -o ${OUTDIR}/peerswap ./cmd/peerswap-plugin
	chmod a+x out/peerswap

${TEST_BIN_DIR}/peerswapd:
	go build ${TEST_BUILD_OPTS} -o ${TEST_BIN_DIR}/peerswapd ./cmd/peerswaplnd/peerswapd
	chmod a+x ${TEST_BIN_DIR}/peerswapd

${TEST_BIN_DIR}/pscli:
	go build ${TEST_BUILD_OPTS} -o ${TEST_BIN_DIR}/pscli ./cmd/peerswaplnd/pscli
	chmod a+x ${TEST_BIN_DIR}/pscli

${TEST_BIN_DIR}/peerswap:
	go build ${TEST_BUILD_OPTS} -o ${TEST_BIN_DIR}/peerswap ./cmd/peerswap-plugin
	chmod a+x ${TEST_BIN_DIR}/peerswap

# Test section. Has commads for local and ci testing.
test:
	PAYMENT_RETRY_TIME=5 go test -tags dev -tags fast_test -timeout=10m -v ./...
.PHONY: test

test-integration: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} ./test
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} ./lnd
.PHONY: test-integration

test-bitcoin-cln: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_ClnCln_Bitcoin_SwapOut|'\
	'Test_ClnCln_Bitcoin_SwapIn|'\
	'Test_ClnLnd_Bitcoin_SwapOut|'\
	'Test_ClnLnd_Bitcoin_SwapIn|'\
	'Test_ClnCln_ExcessiveAmount|'\
	'Test_ClnCln_StuckChannels)'\
	 ./test
.PHONY: test-bitoin-cln

test-bitcoin-lnd: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_LndLnd_Bitcoin_SwapOut|'\
	'Test_LndLnd_Bitcoin_SwapIn|'\
	'Test_LndCln_Bitcoin_SwapOut|'\
	'Test_LndCln_Bitcoin_SwapIn|'\
	'Test_LndLnd_ExcessiveAmount)'\
	 ./test
	${INTEGRATION_TEST_ENV} go test $(INTEGRATION_TEST_OPTS) ./lnd
.PHONY: test-bitcoin-lnd

test-liquid-cln: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_ClnCln_Liquid_SwapOut|'\
	'Test_ClnCln_Liquid_SwapIn|'\
	'Test_ClnLnd_Liquid_SwapOut|'\
	'Test_ClnLnd_Liquid_SwapIn)'\
	 ./test
.PHONY: test-liquid-cln

test-liquid-lnd: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_LndLnd_Liquid_SwapOut|'\
	'Test_LndLnd_Liquid_SwapIn|'\
	'Test_LndCln_Liquid_SwapOut|'\
	'Test_LndCln_Liquid_SwapIn)'\
	 ./test
.PHONY: test-liquid-lnd

test-misc-integration: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} \
	-run '^('\
	'Test_OnlyOneActiveSwapPerChannelCln|'\
	'Test_OnlyOneActiveSwapPerChannelLnd|'\
	'Test_GrpcReconnectStream|'\
	'Test_GrpcRetryRequest|'\
	'Test_RestoreFromPassedCSV|'\
	'Test_Recover_PassedSwap_BTC|'\
	'Test_Recover_PassedSwap_LBTC|'\
	'Test_Wumbo|'\
	'Test_ClnConfig|'\
	'Test_ClnPluginConfigFile|'\
	'Test_ClnPluginConfigFile_DoesNotExist|'\
	'Test_ClnPluginConfig_ElementsAuthCookie|'\
	'Test_ClnPluginConfig_DisableLiquid)'\
	 ./test
.PHONY: test-misc-integration

# Release section. Has the commands to install binaries into the distinct locations.
lnd-release: clean-lnd
	go install -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/peerswapd
	go install -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswaplnd/pscli
.PHONY: lnd-release

cln-release: clean-cln
	# peerswap binary is not installed in GOPATH because it must be called by full pathname as a CLN plugin.
	# You may choose to install it to any location you wish.
	go build -o peerswap -ldflags "-X main.GitCommit=$(GIT_COMMIT)" ./cmd/peerswap-plugin/main.go
.PHONY: cln-release

clean-cln:
	# PeerSwap CLN builds
	rm -f peerswap
	rm -f out/peerswap
	# Purge pre-rename binaries
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

fmt:
	gofmt -l -w -s .
.PHONY: fmt

TOOLS_DIR := ${CURDIR}/tools

.PHONY: tool
tool:
	## Install an individual dependent tool.
	@cd $(TOOLS_DIR) && env GOBIN=$(TOOLS_DIR)/bin go install -trimpath github.com/golangci/golangci-lint/cmd/golangci-lint

.PHONY: clean
clean:  ## clean project directory.
	env GOBIN=${TOOLS_DIR}/bin && @rm -rf ${GOBIN} $(TOOLS_DIR)/bin


.PHONY: lint
lint: lint/golangci-lint
lint: ## Lint source.

.PHONY: lint/golangci-lint
lint/golangci-lint: ## Lint source with golangci-lint.
	$(TOOLS_DIR)/bin/golangci-lint version
	${CURDIR}/tools/bin/golangci-lint run -v

.PHONY: lint/fix
lint/fix: ## Lint and fix source.
	@${MAKE} lint/golangci-lint args='--fix'
