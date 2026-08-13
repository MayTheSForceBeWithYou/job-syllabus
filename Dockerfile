# Multi-stage build producing any one of the five cmd/ binaries, selected by
# --build-arg BINARY=ingest (default). Not wired into a deploy path yet —
# that's Jenkins/ECR/ECS territory starting Phase 2 (docs/design.md §0.4).
FROM golang:1.23-bookworm AS build
ARG BINARY=ingest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/bin ./cmd/${BINARY}

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/bin /bin/app
COPY data/ /data/
ENTRYPOINT ["/bin/app"]
