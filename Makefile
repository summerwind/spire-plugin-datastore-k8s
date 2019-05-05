build: generate test
	go build .

test:
	go vet ./...
	go test -v ./...

generate:
	go generate ./pkg/...

.PHONY: test
