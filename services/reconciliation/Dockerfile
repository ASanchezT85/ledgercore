# syntax=docker/dockerfile:1
# Contexto de build = RAÍZ del monorepo (necesita libs/go y este servicio):
#   docker build -f services/reconciliation/Dockerfile -t ledgercore/reconciliation .

FROM golang:1.26-alpine AS build
WORKDIR /src

# Módulo compartido primero para aprovechar la caché de capas.
COPY libs/go/ libs/go/
COPY services/reconciliation/ services/reconciliation/

WORKDIR /src/services/reconciliation
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/reconciliation ./cmd/reconciliation

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/reconciliation /reconciliation
EXPOSE 8083
USER nonroot
ENTRYPOINT ["/reconciliation"]
