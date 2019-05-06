VERSION=0.1.0

build: generate test
	go build .

test:
	go vet ./...
	go test -v ./...

generate:
	go generate ./pkg/...

build-container:
	docker build -t summerwind/spire-server:latest -t summerwind/spire-server:$(VERSION) .

push-container:
	docker push summerwind/spire-server:latest

push-release-container:
	docker push summerwind/spire-server:$(VERSION)

release:
	hack/release.sh

clean:
	rm -rf spire-plugin-datastore-k8s release

.PHONY: test
