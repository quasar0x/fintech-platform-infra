# ---------- build stage ----------
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install CA certs & git (some modules may require git)
RUN apk add --no-cache ca-certificates git

# Cache deps first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build a static binary (no libc dependency)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/api-gateway ./cmd/api-gateway

# ---------- runtime stage ----------
# Distroless, nonroot, includes CA certs
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /out/api-gateway /api-gateway

# Distroless nonroot uses uid 65532 by default
USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/api-gateway"]
