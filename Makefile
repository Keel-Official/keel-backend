.PHONY: up down psql migrate build test vet fmt arch conformance store-test manual-check ci api-mocks api-mocks-check record record-once record-holders record-batch survey assets scan serve backtest replay crosscheck divergence

# ---------------------------------------------------------------- Local

up:
	docker compose up -d
	@echo "Postgres ready on localhost:5433 (user keel / db keel)"

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
# THE BUILD TAG CAME OUT ON 26 AUGUST 2026, so this suite now also runs inside
# plain `make test` and inside CI. This target is no longer the only way to reach
# it and is kept for two reasons: it runs the package alone and verbosely, which is
# what you want on the day the fixture is the thing under discussion, and DEC-004
# phrases the trigger for opening this repository as "make conformance passes
# without a build tag", which is not checkable if the target does not exist.
#
# golden_test.go's own removal instruction said to delete this target. That one
# line of it was not followed, and the reason is recorded in that file rather than
# only here.
conformance:
	go test ./internal/conformance/ -count=1 -v

# store-test runs the internal/store integration tests against a real Postgres.
# They are SKIPPED by plain `make test`, because they need a database and a
# suite that cannot run without one is a suite that stops being run.
#
# Every test runs inside a transaction that is rolled back, so this leaves no
# rows behind. It needs `make up && make migrate` first.
#
# THE DEFAULT NAMES 5433, which is where docker-compose.yml publishes the
# container as of 26 August 2026. It named 5432 before that, and on a machine
# with a Postgres already installed that address is the HOST server: it binds
# 127.0.0.1 while Docker binds the wildcard, so the symptom is `role "keel" does
# not exist` rather than a refused connection. The ports block in
# docker-compose.yml carries the full account.
#
# It stays overridable, because moving the published port narrows that collision
# and does not close it. Anything already on 5433 wins the same way.
KEEL_TEST_DSN ?= postgres://keel:keel_dev_only@localhost:5433/keel?sslmode=disable

# THE PREFLIGHT RUNS FIRST AND ALONE, and make stops here when it fails. On 26
# August 2026 a DSN naming 5432 reached the host's Postgres instead of the
# container and all 31 tests failed with the same connection error, which reads
# like a broken package rather than a misdirected client. One diagnosis is worth
# more than 31 symptoms.
#
# It is a Go test and not a psql or nc probe: psql is not guaranteed to be on the
# host, and a TCP probe would have PASSED that day, because the port was open and
# a Postgres was answering it. See the comment on the test itself.
store-test:
	@KEEL_TEST_DSN="$(KEEL_TEST_DSN)" go test ./internal/store/ -count=1 -run "^TestPreflight"
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

# manual-check reports how many of the Layer 1 hand recomputations exist under
# testdata/manual/ and, for each one, whether it names an asset and a ledger
# sequence. The required count is read out of docs/methodology/10-validation.md
# section 1, so this target has no number of its own to be edited.
#
# IT EXITS NON-ZERO TODAY, and it is meant to: 0 of 5 exist. Acceptance criterion 4
# is worth 10 of 100, and the absence gets a command rather than a paragraph
# because a criterion that has not moved since 28 August needs a number somebody
# can run, not a note somebody can skim.
#
# CORRECTED 3 SEPTEMBER 2026. The clause "and PRD section 12 lists it among the four
# that are never cuttable" was here and is false: that list is the Blend backtest,
# the methodology document, the cross-validation, and the limitations section, and
# Layer 1 is on none of them. Same wrong sentence in
# scripts/check-manual-recomputation.sh and scripts/audit-verification.sh, corrected
# the same day. Layer 1 is protected by an acceptance criterion and not by the list
# that governs cutting under pressure, which makes this target more worth running
# rather than less.
#
# IT IS DELIBERATELY NOT IN `ci` BELOW, and this is a decision with an end
# condition rather than an exemption. `make ci` is the gate that is expected to be
# green, and a gate that is permanently red stops being read: that is the lesson
# this repository already paid for with the conformance job, which sat in CI with
# continue-on-error for days and was a job nobody was expected to look at. The
# standing report of this absence is P2-23 in scripts/audit-verification.sh, and CI
# runs this script in its own job on every push.
#
# PROMOTION CONDITION: when this target exits 0, move it into `ci` and delete this
# paragraph. That is the same shape of instruction the conformance job carried, and
# it is written here so that the temporary allowance has an owner and an end.
manual-check:
	@bash scripts/check-manual-recomputation.sh

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

# record-holders adds the trustline holder distribution of every BASE asset to
# one round. Separate from record-once rather than a flag on it, because it is
# the one reading here whose request cost grows with the asset: one request per
# 200 accounts, against an hourly budget shared with everything else.
#
# HOLDER_PAGES caps one reading, in pages of 200 accounts, and 0 leaves the
# binary's default of 25, which is 5000 accounts. CHECK THE ASSET FIRST, because
# a reading that hits the cap is written and flagged TRUNCATED, and a truncated
# reading answers a holder COUNT as a lower bound and a concentration question
# not at all: the account it did not read may be the largest one. Horizon's own
# figure is one request away and needs no key:
#
#   curl -s "https://horizon.stellar.org/assets?asset_code=CODE&asset_issuer=ISSUER" \
#     | python3 -c "import json,sys; print(json.load(sys.stdin)['_embedded']['records'][0]['accounts'])"
HOLDER_PAGES ?= 0

record-holders:
	go run ./cmd/keel record -pairs $(PAIRS) -once -holders -holder-pages $(HOLDER_PAGES)

# record-batch is the same-hour cross-check, and it is the one that tests the
# attribution rather than assuming it.
#
# WHAT IT IS FOR. The first Layer 3 run compared the sixty committed recordings
# against books rebuilt about seven hours later: 37 match, 0 mismatch, 23 partial.
# All 23 are attributed to those seven hours, in 10-validation.md, in the evidence
# note and in the README, and nothing has tested that attribution. Every row in
# that run had the SAME delay, so it holds no contrast to test it with. This target
# records a batch and compares it inside the same hour, which is the second arm.
#
# CROSSCHECK_AFTER is the variable under test. The binary's default is five
# minutes and its ceiling is one hour; both are printed at startup and the measured
# gap is stamped on every row and written to the CSV, so two batches at two delays
# can be put side by side.
#
# READ THE TWO NUMBERS TOGETHER AND STOP THERE. One batch is one sample. A partial
# rate quoted without the delay it was measured at is the sentence this target
# exists to make checkable.
#
# It writes recordings under RECORD_OUT, which is gitignored: a batch recorded to
# test the instrument is not the archive the deliverable is checked against, and
# only recordings/samples/ is that. Around 900 Horizon requests for the sixty pair
# demonstration set, against an hourly budget of 3000.
CROSSCHECK_AFTER ?= 5m
CROSSCHECK_OUT ?= measurements/crosscheck/samehour-$(shell date -u +%Y-%m-%dT%H%M%SZ).csv
RECORD_OUT ?= measurements/recordings

# THE PAIR LIST DEFAULTS TO THE SIXTY AND NOT TO $(PAIRS), which is the one place
# this target departs from every other one here. PAIRS defaults to the single-pair
# example, and a batch of one cannot be set beside a batch of sixty: the two rates
# would differ by sampling before they differed by delay. An explicit
# `make record-batch PAIRS=...` on the command line is still honoured, which is
# what $(origin) distinguishes.
BATCH_PAIRS ?= $(if $(filter command line,$(origin PAIRS)),$(PAIRS),configs/demonstration-set.json)

record-batch:
	@mkdir -p $(dir $(CROSSCHECK_OUT)) $(RECORD_OUT)
	go run ./cmd/keel record -pairs $(BATCH_PAIRS) -once \
		-out $(RECORD_OUT) \
		-crosscheck -crosscheck-after $(CROSSCHECK_AFTER) -crosscheck-out $(CROSSCHECK_OUT)

# survey asks Horizon four cheap questions about every pair in PAIRS and prints one
# row each, so the four liquidity buckets in 10-validation.md section 3 are filled
# against numbers. It is a TRIAGE INSTRUMENT: nothing it prints is a Keel output and
# none of it may be quoted as one. It needs no database and writes nothing.
survey:
	bash scripts/candidate-survey.sh $(PAIRS)

# assets declares the demonstration set from the same pair file the recorder
# reads, then lists what is in the table. Needs the database.
assets:
	go run ./cmd/keel assets -pairs $(PAIRS)

scan:
	go run ./cmd/keel scan

serve:
	go run ./cmd/keel serve

# backtest writes the trade-implied history of a pair as two CSV files: one row
# per trade and one row per UTC day. It needs no database and no BigQuery.
#
# IT IS NOT A REPLAY. Horizon serves the whole trade stream of a pair and no
# order book at a past ledger, so this can bound depth from trades that happened
# and cannot see the offers that were merely posted. On USTRY in February 2026
# that difference is the whole of Deliverable 2's claim, and it is measured in
# docs/evidences/2026-08-26-ustry-february-trades-implied.md rather than argued.
#
# FROM_LEDGER is a SEEK, not a boundary. Arriving early costs a few requests;
# arriving late silently drops the front of the window, so the command prints the
# first trade it kept. Read the close time off Horizon rather than computing it:
#
#   curl -s https://horizon.stellar.org/ledgers/60977383 \
#     | python3 -c "import json,sys; print(json.load(sys.stdin)['closed_at'])"
FROM ?= 2026-02-01
TO ?= 2026-03-01
MARK ?= 2026-02-22
FROM_LEDGER ?= 60977383

backtest:
	go run ./cmd/keel backtest -pairs $(PAIRS) -from $(FROM) -to $(TO) -mark $(MARK) -from-ledger $(FROM_LEDGER)

# replay rebuilds a pair's order book at a past ledger from the operations that
# posted it. No database, no BigQuery, and NOT a historical Horizon endpoint:
# Horizon serves no past state and every past event, and a book is what the
# events left behind. See internal/horizon/replay.go and DEC-002 section 2.3.
#
# READ THE COMPLETENESS LINE IT PRINTS. The reconstruction has three ways to be
# incomplete and a missing offer reads as a THIN book, which is this product's
# most interesting finding and so the worst thing to produce by accident.
#
# SINCE_LEDGER is the floor on each account's backwards walk. Raising it makes a
# run cheap and makes every offer created below it invisible; 0 walks each
# account to its first operation. LEDGER is the target and has no default,
# because which ledger to rebuild is a question and not a setting.
LEDGER ?=
SINCE_LEDGER ?= 0
TRADES_FROM ?= 0

replay:
	@test -n "$(LEDGER)" || { echo "replay: LEDGER is required, e.g. make replay PAIRS=... LEDGER=61340262"; exit 1; }
	go run ./cmd/keel replay -pairs $(PAIRS) -ledger $(LEDGER) -since-ledger $(SINCE_LEDGER) -trades-from-ledger $(TRADES_FROM)

# crosscheck is Layer 3 of docs/methodology/10-validation.md, executed. It reads
# the committed recordings, rebuilds each book from Horizon today, and compares
# them at the four depths that document names.
#
# IT GETS WEAKER WITH TIME AND THAT IS INHERENT. The rebuild carries a live offer
# back only when the offer has not moved since the target, so every hour that
# passes moves more offers and turns more pairs from comparable into partial. Run
# it soon after the recording, not weeks later.
crosscheck:
	go run ./cmd/keel crosscheck -out docs/evidences/layer3-crosscheck-$(shell date -u +%Y-%m-%d).csv

# divergence measures how far the order book mid sits from the pool spot price
# across the demonstration set, which is what decides case 1 of the reference
# price ladder. It MEASURES: no threshold is recommended and no branch is called
# correct.
#
# The output directory is gitignored. The summary counts are the result and belong
# in a commit message or a decision record; sixty raw Horizon bodies do not.
#
# 180 requests against the hourly budget, three per pair.
divergence:
	go run ./cmd/keel divergence -pairs $(or $(PAIRS),configs/demonstration-set.json) -out $(or $(OUT),measurements/divergence)
