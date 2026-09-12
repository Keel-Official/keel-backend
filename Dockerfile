# syntax=docker/dockerfile:1
#
# Keel as a container image.
#
# WHAT THIS IS FOR. Deliverable 2's first acceptance criterion reads "FR-18
# through FR-22 live and documented", and PRD section 2.1 says user P3 needs "an
# API they can try in 5 minutes, with no registration". Neither is satisfied by a
# binary that only runs on one laptop.
#
# THE SPLIT THIS FILE SITS ON. Building and publishing an image needs nothing that
# belongs to anybody: the workflow in .github/workflows/deploy.yml pushes to the
# repository's own registry with the token GitHub already provides. Running it
# somewhere needs a hosting account, its secrets, and a database, and those are
# Al's. Same PREPARE and APPLY split as scripts/s3-archive/, and for the same
# reason.
#
# NFR-8 IS ABSOLUTE READ ONLY, and this image cannot change that either way: no
# transaction signing or submission code exists anywhere in the repository, so
# there is nothing here to lock down that the source does not already exclude.
# What the image DOES contribute is a smaller blast radius by construction, which
# is the reason for the base image below.

# ---------------------------------------------------------------- build

FROM golang:1.23-alpine AS build

WORKDIR /src

# Dependencies first, so that a change to the Go sources does not re-download the
# module cache. go.sum is copied with go.mod because `go mod download` verifies
# against it and a missing sum turns a reproducible build into a resolving one.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# THE SAME ARG IS DECLARED IN BOTH STAGES ON PURPOSE. An ARG is scoped to the
# stage that declares it, so the one below the runtime FROM does not reach up
# here. The runtime copy feeds the OCI label; this one goes into the binary, and
# both are fed the same value by the workflow. Declaring it once and expecting it
# in both places is the quiet failure: the label would carry the revision and the
# binary would report "unknown".
ARG VCS_REF=unknown

# CGO_ENABLED=0 IS NOT AN OPTIMISATION, IT IS WHAT MAKES THE RUNTIME STAGE
# POSSIBLE. A cgo binary links against the builder's libc and will not start on a
# distroless static base. pgx is pure Go, so nothing in this repository needs cgo.
#
# -trimpath removes the builder's absolute paths from the binary, which is one
# fewer thing that differs between two builds of the same commit. NFR-9 is about
# the numbers rather than the binary, but the same instinct applies.
#
# -X main.buildRevision IS THE ONLY THING IN THIS BUILD THAT DIFFERS BETWEEN TWO
# COMMITS OF IDENTICAL SOURCE, and that is deliberate rather than a break with
# -trimpath above. GET /v1/health reports it so that a running deployment can be
# asked which commit it is, instead of that being inferred from a tag list. The
# header of cmd/keel/main.go carries why it is a linker stamp and not an
# environment variable.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.buildRevision=${VCS_REF}" \
      -o /out/keel ./cmd/keel

# ---------------------------------------------------------------- runtime

# distroless static: no shell, no package manager, no libc, and it runs as a
# non-root user by default. It carries the CA certificate bundle, which the image
# does need: every subcommand except `serve` reads Horizon over HTTPS, and a
# missing bundle fails as a TLS error at the first request rather than at build
# time.
FROM gcr.io/distroless/static-debian12:nonroot

# Labels rather than a version baked into the binary. `keel version` reports the
# METHODOLOGY version, which is a different thing from which commit built this,
# and conflating the two would put a build stamp in the place a reader looks for
# the methodology. The workflow fills these in.
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="keel" \
      org.opencontainers.image.description="Liquidity risk engine for Stellar assets. Read only." \
      org.opencontainers.image.source="https://github.com/Keel-Official/keel-backend" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.licenses="NOASSERTION"

COPY --from=build /out/keel /usr/local/bin/keel

# 3000 is the default in cmd/keel/serve.go. EXPOSE documents it and publishes
# nothing on its own.
EXPOSE 3000

USER nonroot:nonroot

# No shell exists in this image, so ENTRYPOINT is the exec form and every
# argument reaches the binary directly. `docker run keel bookseries ...` works
# because the entrypoint is the binary rather than one subcommand of it.
ENTRYPOINT ["/usr/local/bin/keel"]
CMD ["serve"]
