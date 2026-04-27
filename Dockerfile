FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/server \
    ./cmd/api

FROM alpine:3.19

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S app && \
    adduser -S app -G app

WORKDIR /app

COPY --from=build /app/server ./server

RUN chown -R app:app /app

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./server"]
