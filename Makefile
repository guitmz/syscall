.PHONY: prepare build test clean

prepare:
	python3 scripts/prepare-dataset.py

build: prepare
	go build -o syscall .

test:
	go vet ./...
	go test ./...

clean:
	rm -rf syscall dist
