.PHONY: run build tidy

run:
	CGO_ENABLED=0 go run ./cmd

build:
	CGO_ENABLED=0 go build -o bin/torra ./cmd

tidy:
	go mod tidy
