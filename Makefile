BINARY_NAME ?= coinbasevwap
FEED_URL    ?= wss://ws-feed.exchange.coinbase.com

.PHONY: all build help lint test run debug

all: build

build:	## Build application
build:
	go build -o ${BINARY_NAME}

help: pad = 24 # padding for two columns
help:	## Show this help
	@echo
	@echo "Coinbase Rate VWAP."
	@echo
	@echo "Commands:"
	@fgrep -h "##" $(MAKEFILE_LIST) \
		| fgrep -v fgrep \
		| sed -e 's/^/  /' -e 's/:/ /' -e 's/	//g' \
		| sort -k 1 \
		| grep -v '^  #' \
		| awk -F "#" '{printf ("%s% *s%s\n", $$1, $(pad)-length($$1), "", $$3)}'
	@echo

lint:	## Start linter
lint:
	golangci-lint run

test:	## Run tests
test:
	go test -timeout 30s -cover ./...

run:	## Start app
run:
	go run main.go -feed-url ${FEED_URL}

debug:	## Start app with debug logging
debug:
	go run main.go -log-level debug -feed-url ${FEED_URL}
