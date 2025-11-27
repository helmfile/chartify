.PHONY: all
all: build

.PHONY: build
build: build-chartify build-chartreposerver

.PHONY: build-chartify
build-chartify:
	go build -o chartify ./cmd/chartify

.PHONY: build-chartreposerver
build-chartreposerver:
	go build -o chartreposerver ./cmd/chartreposerver

.PHONY: install
install:
	go install ./cmd/chartify
	go install ./cmd/chartreposerver

.PHONY: clean
clean:
	rm -f chartify chartreposerver

.PHONY: test
test:
	go test ./...

.PHONY: test/verbose
test/verbose:
	RETAIN_TEMP_DIR=1 go test -v ./...

.PHONY: act
act:
	act
