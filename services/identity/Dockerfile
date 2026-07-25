# syntax=docker/dockerfile:1
# Build context MUST be the repository root:
#   docker build -f services/identity/Dockerfile .

FROM golang:1.26-alpine AS build
WORKDIR /src

# libs/go is consumed via a replace directive, so it must be in the context.
COPY libs/go/ libs/go/
COPY services/identity/go.mod services/identity/go.sum services/identity/
WORKDIR /src/services/identity
RUN go mod download

COPY services/identity/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/identity ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/identity /identity
EXPOSE 8082
USER nonroot
ENTRYPOINT ["/identity"]
