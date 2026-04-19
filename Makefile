.PHONY: run dev build test docs docs-check migrate-up migrate-down lint

run:
	go run ./cmd/api

dev:
	air

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

docs:
	swag init -g cmd/api/main.go -o api/docs --parseInternal -st

docs-check:
	@swag init -g cmd/api/main.go -o api/docs --parseInternal -st
	@git diff --exit-code api/docs || (echo "docs drift detected, run 'make docs' and commit" && exit 1)

migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down 1

lint:
	go vet ./...