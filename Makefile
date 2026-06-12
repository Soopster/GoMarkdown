APP     := markdownviewer
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test bench lint clean

build:
	go build $(LDFLAGS) -o $(APP) .

test:
	go test -race -count=1 ./...

bench:
	go test -bench=. -benchmem -count=3 ./...

lint:
	golangci-lint run

clean:
	rm -f $(APP) $(APP).test
