.PHONY: up down psql build test vet fmt arch conformance ci record scan serve

# ---------------------------------------------------------------- Lokal

up:
	docker compose up -d
	@echo "Postgres siap di localhost:5432 (user keel / db keel)"

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

# arch menegakkan kemurnian internal/domain dan internal/depth: tanpa I/O,
# tanpa float, tanpa time.Now, tanpa goroutine. Sudah termasuk di `make test`,
# tetapi dipisah agar bisa dijalankan sendiri saat menulis di zona murni.
arch:
	go test ./internal/domain/ -run TestArch -count=1 -v

# conformance menguji implementasi metodologi terhadap golden fixture USTRY.
#
# SEMENTARA memakai build tag karena internal/depth belum ada isinya. Begitu
# paket itu terisi, hapus baris //go:build di internal/conformance/golden_test.go
# dan hapus target ini; test harus ikut `make test` biasa.
conformance:
	go test -tags conformance ./internal/conformance/ -count=1 -v

ci: vet arch test

# ---------------------------------------------------------------- Jalankan

record:
	go run ./cmd/keel record

scan:
	go run ./cmd/keel scan

serve:
	go run ./cmd/keel serve
