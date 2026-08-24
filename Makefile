.PHONY: up down psql migrate build test vet fmt arch conformance store-test ci api-mocks api-mocks-check record record-once assets scan serve

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

# store-test runs the internal/store integration tests against a real Postgres.
# They are SKIPPED by plain `make test`, because they need a database and a
# suite that cannot run without one is a suite that stops being run.
#
# Every test runs inside a transaction that is rolled back, so this leaves no
# rows behind. It needs `make up && make migrate` first.
#
# KEEL_TEST_DSN is overridable because localhost:5432 is not always the
# container: a Postgres already running on the host takes that port first, and
# the symptom is `role "keel" does not exist` rather than a refused connection.
KEEL_TEST_DSN ?= postgres://keel:keel_dev_only@localhost:5432/keel?sslmode=disable

store-test:
	KEEL_TEST_DSN="$(KEEL_TEST_DSN)" go test ./internal/store/ -count=1 -v

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

# record needs a pair list, and no list is compiled into the binary: which assets
# Keel measures is decision D-1 and docs/methodology/02-pair-selection.md is
# still a worksheet. PAIRS points at one. The default is an EXAMPLE holding the
# single pair this deliverable is already about; copy it before extending it, and
# keep the working list untracked.
PAIRS ?= scripts/record-pairs.example.json

record:
	go run ./cmd/keel record -pairs $(PAIRS)

# record-once is the one to run first. It records a single round and exits, so a
# newcomer sees a real file appear without committing to a long-running process.
record-once:
	go run ./cmd/keel record -pairs $(PAIRS) -once

# assets declares the demonstration set from the same pair file the recorder
# reads, then lists what is in the table. Needs the database.
assets:
	go run ./cmd/keel assets -pairs $(PAIRS)

scan:
	go run ./cmd/keel scan

serve:
	go run ./cmd/keel serve
