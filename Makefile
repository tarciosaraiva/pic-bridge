.PHONY: build test clean

build:
	go build -o picb ./cmd/picb

test:
	go vet ./...
	go test -race ./...

clean:
	rm -f picb
