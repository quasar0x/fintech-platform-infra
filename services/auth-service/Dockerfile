# --- builder ---
FROM golang:1.24.11-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

# Cache deps.
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary (good for distroless)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/auth-service .

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/auth-service /auth-service

USER nonroot:nonroot
EXPOSE 8081

ENTRYPOINT ["/auth-service"]