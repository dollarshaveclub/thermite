.PHONY: thermite check install dist clean

thermite:
	go build ./...

check:
	go fmt ./...
	go vet ./...
	go test ./...

install:
	go install ./...

IMAGE ?= dollarshaveclub/thermite
TAG ?= latest

dist:
	docker build --tag=${IMAGE}:${TAG} .

clean:
	go clean -i ./...
