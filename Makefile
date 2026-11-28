# Everything runs in Docker. `make` with no target lists the commands.
#   make up        start the dev stack      make down      stop it (data is kept)
#   make logs s=backend                     make prod-up   production stack on a server

COMPOSE      ?= docker compose
PROD         := $(COMPOSE) -f docker-compose.prod.yml
GO_IMAGE     := golang:1.26-alpine
GO_RUN       := docker run --rm -v "$(CURDIR)/backend":/src -v vaultly-gomod:/go/pkg -w /src $(GO_IMAGE)
s            ?=

.DEFAULT_GOAL := help
.PHONY: help demo up down restart build ps logs psql redis test vet fmt check prod-up prod-down prod-ps prod-logs backup

help: ## show this list
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## ---- development ----------------------------------------------------------------------------

demo: ## fresh clone -> running, seeded app in one command (only needs Docker)
	@sh deploy/demo.sh

up: ## start the dev stack (builds images, waits for the backend)
	$(COMPOSE) up -d --build
	@echo "app: http://localhost:3000   api: http://localhost:8080   mail: http://localhost:8025"

down: ## stop the dev stack; volumes (your data) are kept
	$(COMPOSE) down

restart: ## rebuild and restart one service:  make restart s=backend
	$(COMPOSE) up -d --build $(s)

build: ## build images without starting
	$(COMPOSE) build $(s)

ps: ## list containers
	$(COMPOSE) ps

logs: ## follow logs (all, or one):  make logs s=backend
	$(COMPOSE) logs -f --tail=100 $(s)

psql: ## open a psql shell on the app database
	$(COMPOSE) exec postgres psql -U vaultly vaultly

redis: ## open redis-cli
	$(COMPOSE) exec redis redis-cli

## ---- quality --------------------------------------------------------------------------------

test: ## run the Go unit tests
	$(GO_RUN) go test ./...

vet: ## go vet
	$(GO_RUN) go vet ./...

fmt: ## list files that need gofmt (prints nothing when clean)
	$(GO_RUN) gofmt -l .

check: fmt vet test ## fmt + vet + test, as CI does

## ---- production (run on the server, see docs/14_GO_LIVE_RUNBOOK.txt) ------------------------

prod-up: ## start the production stack
	$(PROD) up -d --build

prod-down: ## stop the production stack; NEVER add -v, it deletes your data
	$(PROD) down

prod-ps: ## list production containers
	$(PROD) ps

prod-logs: ## follow production logs:  make prod-logs s=backend
	$(PROD) logs -f --tail=100 $(s)

backup: ## database dump + OpenBao snapshot (set BACKUP_PASSPHRASE to encrypt)
	sh deploy/backup.sh
