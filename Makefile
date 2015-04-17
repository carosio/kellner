GOPATH=$(shell pwd):$(shell pwd)/vendor


all: force
	env GOPATH="$(GOPATH)" go build -v

test:
	env GOPATH="$(GOPATH)" go test

.PHONY : force
