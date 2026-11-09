COMPOSE := docker compose -p ynov-loans -f deploy/docker-compose.yml
IMAGE ?= ynov-loans-api
REALM ?= loans
CLIENT_ID ?= loans-api
TEST_USER_PASSWORD ?= Loans-Test-2026!

.PHONY: build test lint up down token smoke mcp-add

build:
	docker build -t $(IMAGE) .

test:
	scripts/go.sh go test ./...

lint:
	scripts/go.sh go vet ./...

up:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down -v

token:
ifneq ($(origin USER),command line)
	$(error usage: make token USER=student1|student2|staff1|admin1 (aliases: student, staff, admin))
endif
	@u="$(USER)"; \
	case "$$u" in student) u=student1;; staff) u=staff1;; admin) u=admin1;; esac; \
	curl -fsS -X POST http://localhost:3333/realms/$(REALM)/protocol/openid-connect/token \
	  -d grant_type=password -d client_id=$(CLIENT_ID) \
	  -d "username=$$u" --data-urlencode "password=$(TEST_USER_PASSWORD)" \
	  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p'

smoke:
	scripts/smoke.sh

MCP_USER ?= staff

mcp-add:
	@t="$$($(MAKE) -s token USER=$(MCP_USER))"; \
	[ -n "$$t" ] || { echo "token vide: la stack tourne-t-elle (make up) ?" >&2; exit 1; }; \
	claude mcp remove --scope user loans >/dev/null 2>&1 || true; \
	claude mcp add --scope user --transport http loans http://localhost:8081/ \
	  --header "Authorization: Bearer $$t"
