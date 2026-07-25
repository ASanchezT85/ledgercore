# syntax=docker/dockerfile:1
# Build context: the REPOSITORY ROOT (it must copy libs/go and this service).
#   docker build -f services/ledger-core/Dockerfile -t ledgercore/ledger-core .

FROM golang:1.26-alpine AS build
WORKDIR /src

# Prime the module cache first for better layer reuse.
COPY libs/go/go.mod libs/go/go.sum ./libs/go/
COPY services/ledger-core/go.mod services/ledger-core/go.sum ./services/ledger-core/
WORKDIR /src/services/ledger-core
RUN go mod download

# Now the sources.
WORKDIR /src
COPY libs/go ./libs/go
COPY services/ledger-core ./services/ledger-core
WORKDIR /src/services/ledger-core
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ledger-core ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ledger-core /ledger-core
EXPOSE 8081
USER nonroot
ENTRYPOINT ["/ledger-core"]
