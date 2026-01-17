# --- builder ---
FROM golang:1.24.11-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/payments-service .

# --- ruutime ---
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/payments-service /payments-service

USER nonroot:nonroot
EXPOSE 8083
ENTRYPOINT ["/payments-service"]