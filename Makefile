.PHONY: test
test:
	go test ./...

.PHONY: test/verbose
test/verbose:
	RETAIN_TEMP_DIR=1 go test -v ./...
