GOPATH=$(shell pwd):$(shell pwd)/vendor


all: force
	env GOPATH="$(GOPATH)" go build -v

.PHONY : force
