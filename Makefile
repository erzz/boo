.PHONY: build test test-int lint fmt clean run

BINARY := bin/boo
PKG := ./...

build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/boo

test:
	go test $(PKG)

test-int:
	go test -tags=integration $(PKG)

lint:
	golangci-lint run

fmt:
	gofmt -w .
	command -v goimports >/dev/null && goimports -w . || true

clean:
	rm -rf bin dist

run: build
	./$(BINARY)
