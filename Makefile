.PHONY: help lint test run debug

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

.PHONY: help

lint:	## Start linter
	golangci-lint run

test:	## Run tests
test:
	go test -cover ./...

run:	## Start app
run:
	go run main.go

debug:	## Start app with debug logging
debug:
	go run main.go -log-level debug
