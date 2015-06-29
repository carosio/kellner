GOPATH=$(shell pwd):$(shell pwd)/vendor


all: force
	env GOPATH="$(GOPATH)" go build -v

# create a binary with debug-symbols removed
all-release: force 
	env GOPATH="$(GOPATH)" go build -v -ldflags "-s"

test:
	env GOPATH="$(GOPATH)" go test

.PHONY : force
