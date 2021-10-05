build:
	go build -tags dev -o peerswap ./cmd/peerswap/main.go
	chmod a+x peerswap
.PHONY: build

test: build
	go test -race -count=1 -v ./...
.PHONY: test

test-all: build
	go test -count=1 --tags docker ./...
.PHONY: test-all

release:
	go build -o peerswap ./cmd/peerswap/main.go
.PHONY: release

pytest: build
	pytest ./test

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
	
