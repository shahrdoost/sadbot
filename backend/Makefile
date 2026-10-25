COMPOSE := docker compose -f ../docker-compose.yml

.PHONY: db-up db-down migrate seed backend test vet

db-up:
	$(COMPOSE) up -d

db-down:
	$(COMPOSE) down

migrate:
	migrate -path migrations -database "$(DATABASE_URL)" up

seed:
	go run ./cmd/sadbot -seed-only

backend:
	go run ./cmd/sadbot

test:
	go test ./...

vet:
	go vet ./...
