# Multi-stage build producing any one of the five cmd/ binaries, selected by
# --build-arg BINARY=ingest (default). Never actually built as a real
# container until Phase 3's first real CI run — go.mod had already moved
# to `go 1.26.5` (GOTOOLCHAIN=local in this base image, so no auto-fetch)
# while this stayed pinned to 1.23, so it failed immediately: "go.mod
# requires go >= 1.26.5 (running go 1.23.12)".
FROM golang:1.26-bookworm AS build
ARG BINARY=ingest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/bin ./cmd/${BINARY}

# /app/bin, not /bin/app: ci/Jenkinsfile.api-build's Kaniko invocation
# --ignore-path-protects /bin (among other system dirs) so the live JNLP
# agent process survives Kaniko's between-stage filesystem wipe — an
# ignored path is excluded from Kaniko's snapshot diff entirely, so a
# build artifact placed under it would silently vanish from the pushed
# image. Keeping our own output outside every ignored path sidesteps that.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/bin /app/bin
COPY data/ /data/
ENTRYPOINT ["/app/bin"]
