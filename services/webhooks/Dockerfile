# Build context = repo root (must copy libs/go and the service):
#   docker build -f services/webhooks/Dockerfile -t ledgercore/webhooks .
FROM golang:1.26-alpine AS build

WORKDIR /src

# Copy the shared library first (the service go.mod replaces it locally).
COPY libs/go/ libs/go/
COPY services/webhooks/ services/webhooks/

WORKDIR /src/services/webhooks
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/webhooks ./cmd/webhooks

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/webhooks /webhooks

EXPOSE 8084
ENTRYPOINT ["/webhooks"]
