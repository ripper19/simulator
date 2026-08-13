.PHONY: test race vet fmt build bench clean

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

build:
	go build ./...

bench:
	go test -run '^$$' -bench . -benchmem ./...

clean:
	go clean -cache
