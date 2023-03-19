BIN := varnamd
HASH := $(shell git rev-parse HEAD | cut -c 1-8)
COMMIT_DATE := $(shell git show -s --format=%ci ${HASH})
BUILD_DATE := $(shell date '+%Y-%m-%d %H:%M:%S')
VERSION := ${HASH} (${COMMIT_DATE})
STATIC := ui:/

deps:
	go get -u github.com/knadh/stuffbin/...

ui/embed.js:
	git clone git@github.com:varnamproject/webpage-embed-plugin.git --depth 1 || true
	cd webpage-embed-plugin && yarn && yarn build && cp dist/embed.cjs ../ui/embed.js

build: ## Build the binary (default)
	go build -o ${BIN} -ldflags="-X 'main.buildVersion=${VERSION}' -X 'main.buildDate=${BUILD_DATE}' -s -w"
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

.PHONY: run
run:
	./${BIN}

.PHONY: clean
clean: ## Remove temporary files and the binary
	go clean
	rm -rf webpage-embed-plugin

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build