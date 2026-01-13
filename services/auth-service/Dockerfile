# --- build ---
FROM golang:1.22-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/auth-service ./cmd/auth-service

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/auth-service /auth-service
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/auth-service"]
