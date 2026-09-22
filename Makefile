.PHONY: test vet fmt-check postgres-up migrate-up migrate-down run

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

postgres-up:
	docker compose up -d postgres

migrate-up:
	docker compose --profile tools run --rm migrate up

migrate-down:
	docker compose --profile tools run --rm migrate down 1

run:
	go run ./cmd/server
