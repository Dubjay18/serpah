.PHONY: up down test lint build-all migrate-up migrate-down keys swagger

up:
	docker compose up --build -d

down:
	docker compose down -v

test:
	@set -e; \
	for dir in shared services/auth services/accounts services/ledger services/payments services/gateway; do \
		(cd "$$dir" && go test ./... -race -count=1); \
	done

lint:
	golangci-lint run ./...

build-all:
	go build ./services/...

migrate-up:
	@for svc in auth accounts ledger payments; do \
		echo "Migrating $$svc..."; \
		goose -dir services/$$svc/migrations postgres "$$DATABASE_URL" up; \
	done

migrate-down:
	@echo "Specify service: make migrate-down SVC=ledger"
	goose -dir services/$(SVC)/migrations postgres "$$DATABASE_URL" down

rabbitmq-status:
	docker compose exec rabbitmq rabbitmqctl status

rabbitmq-reset:
	docker compose restart rabbitmq

keys:
	@mkdir -p keys
	openssl genrsa -out keys/private.pem 2048
	openssl rsa -in keys/private.pem -pubout -out keys/public.pem
	@echo "RS256 keypair written to ./keys/"

swagger: ## Regenerate OpenAPI specs for all services (requires swag in PATH)
	@export PATH=$$PATH:$$HOME/go/bin; \
	for svc in auth accounts ledger payments gateway; do \
		echo "==> swag init: $$svc"; \
		(cd services/$$svc/cmd/server && swag init \
			-g main.go \
			-o ../../docs \
			--dir .,../../internal/handler \
			--parseDependency \
			--quiet); \
	done

auth-mod-tidy:
	@cd services/auth && go mod tidy

# any microservice go get $(packages) && go mod tidy
any-mod-tidy:
	@for dir in services/auth services/accounts services/ledger services/payments services/gateway; do \
		(cd "$$dir" && go get $(packages) && go mod tidy); \
	done

mod-tidy:
	@echo "Specify service: make mod-tidy SVC=ledger"
	@cd services/$(SVC) && go mod tidy

get:
	@echo "Specify service: make get SVC=ledger packages=github.com/some/package"
	@cd services/$(SVC) && go get $(packages) && go mod tidy