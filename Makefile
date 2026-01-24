.PHONY: test
test:
	go test ./...

.PHONY: test/verbose
test/verbose:
	RETAIN_TEMP_DIR=1 go test -v ./...

.PHONY: build
build:
	go build -o chartify ./cmd/chartify

.PHONY: build/chartreposerver
build/chartreposerver:
	go build -o chartreposerver ./cmd/chartreposerver

.PHONY: act
act:
	act
