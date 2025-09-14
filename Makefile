.PHONY: run tidy build

run:
	go run ./cmd/bot

tidy:
	go mod tidy

build:
	go build -o bin/bot ./cmd/bot
