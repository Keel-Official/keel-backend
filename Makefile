.PHONY: up down psql migrate build test vet fmt arch conformance ci api-mocks api-mocks-check record scan serve

# ---------------------------------------------------------------- Local

up:
	docker compose up -d
	@echo "Postgres ready on localhost:5432 (user keel / db keel)"

down:
	docker compose down

psql:
	docker compose exec postgres psql -U keel -d keel

# migrate applies every file in migrations/ in filename order, exactly once
# each, tracked in a schema_migrations table. It is the ONLY way migrations are
# applied; see the comment in docker-compose.yml for why initdb is not used.
migrate:
	bash scripts/migrate.sh

# ---------------------------------------------------------------- Go

build:
	go build ./...

test:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# arch enforces the purity of internal/domain: no I/O, no float, no time.Now, no
# goroutines. It already runs inside `make test`, but it is split out so it can be
# run on its own while writing in a pure package.
arch:
	go test ./internal/domain/ -run TestArch -count=1 -v

# conformance tests the methodology implementation against the USTRY golden
# fixture.
#
# It TEMPORARILY uses a build tag because every function in
# internal/domain/compute.go panics. Once that file is filled in, delete the
# //go:build line in internal/conformance/golden_test.go and delete this target;
# the test belongs in a plain `make test`.
conformance:
	go test -tags conformance ./internal/conformance/ -count=1 -v

# api-mocks writes every example response in the contract out as standalone JSON
# for the frontend. api-mocks-check proves those files still match the contract; a
# non-empty diff means the contract moved and the mocks did not.
api-mocks:
	bash scripts/generate-api-mocks.sh

api-mocks-check:
	@tmp=$$(mktemp -d); \
	bash scripts/generate-api-mocks.sh "$$tmp" >/dev/null; \
	if diff -r -q -x README.md docs/api/mocks "$$tmp" >/dev/null 2>&1; then \
		echo "api-mocks-check: mocks match the contract"; \
	else \
		echo "api-mocks-check: mocks are STALE. Run make api-mocks"; \
		diff -r -x README.md docs/api/mocks "$$tmp" || true; \
		exit 1; \
	fi

ci: vet arch test

# ---------------------------------------------------------------- Run

record:
	go run ./cmd/keel record

scan:
	go run ./cmd/keel scan

serve:
	go run ./cmd/keel serve
