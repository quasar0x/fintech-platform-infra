package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	AppName      string
	Environment  string
	Port         string
	JWTAudience  string
	JWTIssuer    string
	JWTPublicKey *rsa.PublicKey
}

type ctxKeyClaims struct{}

func withClaims(ctx context.Context, claims jwt.MapClaims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims{}, claims)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	mux := http.NewServeMux()

	// root endpoint (useful for scanners/load balancers)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": cfg.AppName,
			"env":     cfg.Environment,
			"routes":  []string{"/healthz", "/readyz", "/v1/ping", "/v1/me"},
		})
	})

	// health endpoints (NO auth)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// In real life: check dependencies (db/cache/queue) For now: always ready
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready",
		})
	})

	// public endpoint
	mux.HandleFunc("/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "pong",
			"app":     cfg.AppName,
			"env":     cfg.Environment,
		})
	})

	// protected endpoint
	mux.Handle("/v1/me", authMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := r.Context().Value(ctxKeyClaims{}).(jwt.MapClaims)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"claims": claims,
		})
	})))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           loggingMiddleware(cfg, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("starting %s (%s) on :%s", cfg.AppName, cfg.Environment, cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() (Config, error) {
	app := getenv("APP_NAME", "api-gateway")
	env := getenv("ENVIRONMENT", "prod")
	port := getenv("PORT", "8080")

	issuer := getenv("JWT_ISSUER", "fintech-auth")
	aud := getenv("JWT_AUDIENCE", "fintech-platform")

	pubKeyPEM := os.Getenv("JWT_PUBLIC_KEY")
	if pubKeyPEM == "" {
		return Config{}, fmt.Errorf("JWT_PUBLIC_KEY is required (PEM encoded RSA public key)")
	}

	pubKey, err := parseRSAPublicKeyFromPEM(pubKeyPEM)
	if err != nil {
		return Config{}, fmt.Errorf("invalid JWT_PUBLIC_KEY: %w", err)
	}

	return Config{
		AppName:      app,
		Environment:  env,
		Port:         port,
		JWTAudience:  aud,
		JWTIssuer:    issuer,
		JWTPublicKey: pubKey,
	}, nil
}

func parseRSAPublicKeyFromPEM(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Expect PKIX "PUBLIC KEY".
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return pub, nil
}

func authMiddleware(cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "missing_bearer_token",
			})
			return
		}

		// "Bearer " is always 7 chars; we already validated the prefix case-insensitively.
		tokenStr := strings.TrimSpace(authz[7:])

		tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			// enforce RS256
			if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
				return nil, fmt.Errorf("unexpected alg: %s", t.Method.Alg())
			}
			return cfg.JWTPublicKey, nil
		},
			jwt.WithAudience(cfg.JWTAudience),
			jwt.WithIssuer(cfg.JWTIssuer),
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		)

		if err != nil || !tok.Valid {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "invalid_token",
			})
			return
		}

		claims, _ := tok.Claims.(jwt.MapClaims)

		ctx := withClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start)

		// Simple JSON-shaped log line (good enough for now; later we can add request IDs)
		log.Printf(`{"app":"%s","env":"%s","method":"%s","path":"%s","duration_ms":%d}`,
			cfg.AppName, cfg.Environment, r.Method, r.URL.Path, d.Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
