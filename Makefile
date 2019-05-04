build:
	go build .

.PHONY: test
test:
	go vet ./...
	go test -v ./...

generate:
	go generate ./pkg/...
