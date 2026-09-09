SHELL := sh

.PHONY: help
.DEFAULT_GOAL := help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' 

.PHONY: build
build: ## build module and CLI
	go build ./...
	go build ./cmd/tojson/...

.PHONY: test
test: ## run all unit tests
	go test ./...

.PHONY: test-toml
test-toml: ## run only TOML tests
	go test -v -run TOML ./...

.PHONY: version
version: ## print OS, Go, and golangci versions
	@echo $$0
	@uname -a
	@go version
	@golangci-lint --version

.PHONY: bench
bench: ## run local benchmarks
	go test -benchmem -bench .

FUZZTIME ?= 30s
FUZZTARGETS = FuzzYAMLLayout FuzzFromYAML FuzzJSONVariant FuzzToYAMLRoundTrip

.PHONY: fuzz
fuzz: ## run each fuzz target (override with FUZZTIME=2m)
	@for t in $(FUZZTARGETS); do \
		echo "=== $$t ($(FUZZTIME))"; \
		go test -run '^$$' -fuzz "^$$t$$" -fuzztime $(FUZZTIME) . || exit 1; \
	done

.PHONY: compare
compare: ## run benchmarks comparing against other libraries
	cd benchmarks && $(MAKE)

.PHONY: cover
cover: ## generate code coverage report
	rm -f cover.out
	go test -run='^Test' -coverprofile=cover.out -coverpkg=.
	go tool cover -func=cover.out

.PHONY: lintverify
## NOTE: this downloads it's schema over the network
lintverify:
	golangci-lint config verify

.PHONY: fmt
fmt: ## reformat source code
	go mod tidy
	gofmt -w -s *.go

.PHONY: lint
lint: ## lint and verify repo is already formatted
	go mod tidy
	git diff --exit-code -- go.mod go.sum
	golangci-lint run .

.PHONY: clean
clean: ## remove any generated files
	rm -f *.out benchmarks/*.out
	rm -f tojson	
	rm -f benchmarks/mem.out
	rm -f benchmarks/benchmarks.test
	rm -f tojson.test
	go clean -fuzzcache


