.PHONY: up down psql build test vet fmt arch conformance ci record scan serve

# ---------------------------------------------------------------- Local

up:
	docker compose up -d
	@echo "Postgres ready on localhost:5432 (user keel / db keel)"

down:
	docker compose down

psql:
	docker compose exec postgres psql -U keel -d keel

# ---------------------------------------------------------------- Go

build:
	go build ./...

test:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# arch enforces the purity of internal/domain and internal/depth: no I/O, no
# float, no time.Now, no goroutines. It already runs inside `make test`, but it
# is split out so it can be run on its own while writing in a pure package.
arch:
	go test ./internal/domain/ -run TestArch -count=1 -v

# conformance tests the methodology implementation against the USTRY golden
# fixture.
#
# It TEMPORARILY uses a build tag because internal/depth has no body yet. Once
# that package is filled in, delete the //go:build line in
# internal/conformance/golden_test.go and delete this target; the test belongs in
# a plain `make test`.
conformance:
	go test -tags conformance ./internal/conformance/ -count=1 -v

ci: vet arch test

# ---------------------------------------------------------------- Run

record:
	go run ./cmd/keel record

scan:
	go run ./cmd/keel scan

serve:
	go run ./cmd/keel serve
