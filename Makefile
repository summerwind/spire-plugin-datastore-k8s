build: generate test
	go build .

test:
	go vet ./...
	go test -v ./...

generate:
	go generate ./pkg/...

build-container:
	docker build -t summerwind/spire-server:latest .

push-container:
	docker push summerwind/spire-server:latest

.PHONY: test
