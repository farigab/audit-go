FROM golang:1.26.2-alpine3.21 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/server \
    ./cmd/audit

FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S app && \
    adduser -S app -G app

WORKDIR /app

COPY --chown=app:app --from=build /app/server ./server

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./server"]
