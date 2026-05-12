BINARY_NAME=evva
BIN_DIR=bin
CMD_DIR=./cmd/evva

.PHONY: all build run test vet fmt tidy clean lint

all: fmt vet test build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

run:
	go run $(CMD_DIR)

test:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR)
