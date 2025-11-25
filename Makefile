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

# Default parallelism for in-package parallel tests (can be overridden via env)
INTEGRATION_TEST_PARALLEL ?= 6

INTEGRATION_TEST_OPTS= \
	-tags dev \
	-tags fast_test \
	-timeout=30m -v \
	-parallel=$(INTEGRATION_TEST_PARALLEL)

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

# Test section. Has commands for local and ci testing.
test:
	PAYMENT_RETRY_TIME=5 go test -tags dev -tags fast_test -race -timeout=10m -v ./...
.PHONY: test

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
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

.PHONY: clean
clean:  ## clean project directory.
	env GOBIN=${TOOLS_DIR}/bin && @rm -rf ${GOBIN} $(TOOLS_DIR)/bin

.PHONY: lint
lint: lint/golangci-lint
lint: ## Lint source.

.PHONY: lint/golangci-lint
lint/golangci-lint: ## Lint source with golangci-lint.
	golangci-lint version
	@BASE_CANDIDATES="$$LINT_BASE origin/main origin/master main master"; \
	for ref in $$BASE_CANDIDATES; do \
		if [ -n "$$ref" ] && git rev-parse --verify "$$ref" >/dev/null 2>&1; then \
			LINT_BASE_REF=$$ref; \
			break; \
		fi; \
	done; \
	if [ -n "$$LINT_BASE_REF" ]; then \
		NEW_FROM_REV=$$(git merge-base "$$LINT_BASE_REF" HEAD); \
		echo "lint: running golangci-lint for changes since $$LINT_BASE_REF (merge-base $$NEW_FROM_REV)"; \
		golangci-lint run -v --new-from-rev "$$NEW_FROM_REV" $(args); \
	else \
		CHANGED_GO_FILES=$$( (git diff --name-only HEAD -- '*.go'; git diff --name-only --cached HEAD -- '*.go') | sort -u ); \
		if [ -z "$$CHANGED_GO_FILES" ]; then \
			echo "lint: no Go changes detected; skipping"; \
		else \
			echo "lint: running golangci-lint for local Go changes"; \
			echo "$$CHANGED_GO_FILES"; \
			golangci-lint run -v $(args) $$CHANGED_GO_FILES; \
		fi; \
	fi

.PHONY: lint/fix
lint/fix: ## Lint and fix source.
	@${MAKE} lint/golangci-lint args='--fix'


.PHONY: mockgen
mockgen: mockgen/lwk

.PHONY: mockgen/lwk
mockgen/lwk:
	$(TOOLS_DIR)/bin/mockgen -source=electrum/electrum.go -destination=electrum/mock/electrum.go

# Matrix-aligned integration targets (for CI and local parity)
.PHONY: test-matrix-bitcoin_clncln
test-matrix-bitcoin_clncln: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(bitcoin_clncln).*$$' ./test

.PHONY: test-matrix-bitcoin_mixed
test-matrix-bitcoin_mixed: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(bitcoin_mixed).*$$' ./test

.PHONY: test-matrix-bitcoin_lndlnd
test-matrix-bitcoin_lndlnd: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(bitcoin_lndlnd).*$$' ./test

.PHONY: test-matrix-liquid_clncln
test-matrix-liquid_clncln: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(liquid_clncln).*$$' ./test

.PHONY: test-matrix-liquid_mixed
test-matrix-liquid_mixed: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(liquid_mixed).*$$' ./test

.PHONY: test-matrix-liquid_lndlnd
test-matrix-liquid_lndlnd: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^Test_Swap(In|Out)Matrix/(liquid_lndlnd).*$$' ./test

# Consolidated misc tests including CLN/LND-specific invariants and setup checks
.PHONY: test-matrix-misc
test-matrix-misc: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^(Test_OnlyOneActiveSwapPerChannelCln|Test_OnlyOneActiveSwapPerChannelLnd|Test_GrpcReconnectStream|Test_GrpcRetryRequest|Test_RestoreFromPassedCSV|Test_Recover_PassedSwap_BTC|Test_Recover_PassedSwap_LBTC|Test_ClnConfig|Test_ClnPluginConfigFile|Test_ClnPluginConfigFile_DoesNotExist|Test_ClnPluginConfig_ElementsAuthCookie|Test_ClnPluginConfig_DisableLiquid|Test_CLNLiquidSetup|Test_ClnCln_ExcessiveAmount|Test_ClnCln_StuckChannels|Test_LndLnd_ExcessiveAmount|Test_Wumbo|Test_Cln_HtlcMaximum|Test_Cln_Premium|Test_Cln_shutdown|Test_ClnCln_Poll)$$' ./test

# Sharded misc tests to reduce single-job runtime in CI
.PHONY: test-matrix-misc_1
test-matrix-misc_1: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^(Test_ClnConfig|Test_ClnPluginConfigFile|Test_ClnPluginConfigFile_DoesNotExist|Test_ClnPluginConfig_ElementsAuthCookie|Test_ClnPluginConfig_DisableLiquid|Test_CLNLiquidSetup|Test_Wumbo)$$' ./test

.PHONY: test-matrix-misc_2
test-matrix-misc_2: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^(Test_GrpcReconnectStream|Test_GrpcRetryRequest|Test_RestoreFromPassedCSV|Test_Recover_PassedSwap_BTC|Test_Recover_PassedSwap_LBTC)$$' ./test

.PHONY: test-matrix-misc_3
test-matrix-misc_3: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} -run '^(Test_OnlyOneActiveSwapPerChannelCln|Test_OnlyOneActiveSwapPerChannelLnd|Test_ClnCln_ExcessiveAmount|Test_ClnCln_StuckChannels|Test_LndLnd_ExcessiveAmount|Test_Cln_HtlcMaximum|Test_Cln_Premium|Test_Cln_shutdown|Test_ClnCln_Poll)$$' ./test

# LND package tests
.PHONY: test-matrix-lnd
test-matrix-lnd: test-bins
	${INTEGRATION_TEST_ENV} go test ${INTEGRATION_TEST_OPTS} ./lnd
