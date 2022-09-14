IMG_WIDTH=1600
IMG_HEIGHT=400

MD_FILES= \
	docs/mmd/swap-in-receiver-states.md \
	docs/mmd/swap-in-sender-states.md \
	docs/mmd/swap-out-receiver-states.md \
	docs/mmd/swap-out-sender-states.md \
	docs/mmd/swap-in-sequence.md \
	docs/mmd/swap-out-sequence.md

DOC_FILES= \
	docs/img/swap-in-receiver-states.png \
	docs/img/swap-in-sender-states.png \
	docs/img/swap-out-receiver-states.png \
	docs/img/swap-out-sender-states.png \
	docs/img/swap-in-sequence.png \
	docs/img/swap-out-sequence.png

.PHONY: subdirs ${MD_FILES}

all-states: ${MD_FILES}

all-docs: all-states ${DOC_FILES}

clean-docs:
	rm -f ${DOC_FILES}
.PHONY: clean-docs

docs/mmd/swap-in-receiver-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-in-receiver-states.md -fsm swap_in_receiver

docs/mmd/swap-in-sender-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-in-sender-states.md -fsm swap_in_sender

docs/mmd/swap-out-receiver-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-out-receiver-states.md -fsm swap_out_receiver

docs/mmd/swap-out-sender-states.md:
	go run ./contrib/stateparser.go -out docs/mmd/swap-out-sender-states.md -fsm swap_out_sender

docs/img/swap-in-receiver-states.png: docs/mmd/swap-in-receiver-states.md
	sed 's/`//g' docs/mmd/swap-in-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-receiver-states.png

docs/img/swap-in-sender-states.png: docs/mmd/swap-in-sender-states.md
	sed 's/`//g' docs/mmd/swap-in-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-in-sender-states.png

docs/img/swap-out-receiver-states.png: docs/mmd/swap-out-receiver-states.md
	sed 's/`//g' docs/mmd/swap-out-receiver-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-receiver-states.png

docs/img/swap-out-sender-states.png: docs/mmd/swap-out-sender-states.md
	sed 's/`//g' docs/mmd/swap-out-sender-states.md | sed 's/mermaid//' | mmdc -w $(IMG_WIDTH) -H $(IMG_HEIGHT) -o docs/img/swap-out-sender-states.png

docs/img/swap-in-sequence.png: docs/mmd/swap-in-sequence.md
	sed 's/`//g' docs/mmd/swap-in-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-in-sequence.png

docs/img/swap-out-sequence.png: docs/mmd/swap-out-sequence.md
	sed 's/`//g' docs/mmd/swap-out-sequence.md | sed 's/mermaid//' | mmdc -o docs/img/swap-out-sequence.png