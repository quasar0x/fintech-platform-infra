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
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Config struct {
	AppName      string
	Environment  string
	Port         string
	JWTAudience  string
	JWTIssuer    string
	JWTPublicKey *rsa.PublicKey

	AuthServiceURL     *url.URL
	PaymentsServiceURL *url.URL
	UserServiceURL     *url.URL

	// When true, api-gateway will proxy based on paths.
	// Example:
	//  /auth/*      -> auth-service (strip /auth)
	//  /payments/*  -> payments-service (strip /payments)
	//  /users/*     -> user-service (strip /users)
	EnableProxy bool
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

	// ---- OpenTelemetry (minimal init) ----
	ctx := context.Background()
	tcfg := loadTelemetryConfig()
	shutdown, err := initTracer(ctx, tcfg)
	if err != nil {
		log.Fatalf("otel init error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	// -------------------------------------

	mux := http.NewServeMux()

	// root endpoint (useful for scanners/load balancers)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": cfg.AppName,
			"env":     cfg.Environment,
			"routes": []string{
				"/healthz",
				"/readyz",
				"/v1/ping",
				"/v1/me",
				"/auth/* (proxy)",
				"/payments/* (proxy)",
				"/users/* (proxy)",
			},
			"proxy_enabled": cfg.EnableProxy,
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
		// In real life: check deps. Here: always ready.
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

	// protected endpoint (local)
	mux.Handle("/v1/me", authMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := r.Context().Value(ctxKeyClaims{}).(jwt.MapClaims)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"claims": claims,
		})
	})))

	// ---------------------------
	// Reverse proxy routes
	// ---------------------------
	if cfg.EnableProxy {
		// Auth service: typically public for register/login/refresh/logout,
		// but you can still leave it public here and secure at auth-service itself.
		authProxy := newReverseProxy("auth-service", cfg.AuthServiceURL)

		// Payments + Users are usually protected => we wrap with JWT middleware.
		paymentsProxy := newReverseProxy("payments-service", cfg.PaymentsServiceURL)
		usersProxy := newReverseProxy("user-service", cfg.UserServiceURL)

		// /auth/* -> auth-service (strip /auth)
		mux.Handle("/auth/", stripPrefixAndProxy("/auth", authProxy))

		// /payments/* -> payments-service (strip /payments) + JWT
		mux.Handle("/payments/", authMiddleware(cfg, stripPrefixAndProxy("/payments", paymentsProxy)))

		// /users/* -> user-service (strip /users) + JWT
		mux.Handle("/users/", authMiddleware(cfg, stripPrefixAndProxy("/users", usersProxy)))
	}

	// Wrap inbound HTTP with OTel and keep your logging middleware
	handler := otelhttp.NewHandler(mux, cfg.AppName)
	handler = loggingMiddleware(cfg, handler)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
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

	// Proxy settings (K8s service DNS names by default)
	enableProxy := strings.ToLower(getenv("ENABLE_PROXY", "true")) == "true"

	authURL, err := mustParseURL(getenv("AUTH_SERVICE_URL", "http://auth-service.fintech-prod.svc.cluster.local"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid AUTH_SERVICE_URL: %w", err)
	}
	payURL, err := mustParseURL(getenv("PAYMENTS_SERVICE_URL", "http://payments-service.fintech-prod.svc.cluster.local"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid PAYMENTS_SERVICE_URL: %w", err)
	}
	userURL, err := mustParseURL(getenv("USER_SERVICE_URL", "http://user-service.fintech-prod.svc.cluster.local"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid USER_SERVICE_URL: %w", err)
	}

	return Config{
		AppName:            app,
		Environment:        env,
		Port:               port,
		JWTAudience:        aud,
		JWTIssuer:          issuer,
		JWTPublicKey:       pubKey,
		AuthServiceURL:     authURL,
		PaymentsServiceURL: payURL,
		UserServiceURL:     userURL,
		EnableProxy:        enableProxy,
	}, nil
}

func mustParseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("url must include scheme and host, got: %q", raw)
	}
	return u, nil
}

func parseRSAPublicKeyFromPEM(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
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

// authMiddleware validates Bearer JWT and injects claims into context.
func authMiddleware(cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing_bearer_token"})
			return
		}

		tokenStr := strings.TrimSpace(authz[7:])

		tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
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
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_token"})
			return
		}

		claims, _ := tok.Claims.(jwt.MapClaims)
		ctx := withClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// newReverseProxy creates a reverse proxy with:
// - safe timeouts
// - OTel-instrumented outbound transport
// - basic error handling
func newReverseProxy(serviceName string, target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Outbound transport (instrumented)
	base := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	proxy.Transport = otelhttp.NewTransport(base)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Preserve original host header? Usually NO for in-cluster services.
		// req.Host = target.Host

		// Add forwarding headers
		if req.Header.Get("X-Forwarded-Proto") == "" {
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		// X-Forwarded-For is handled by Go reverse proxy automatically in many cases,
		// but we ensure it appends.
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf(`{"app":"api-gateway","proxy":"%s","error":%q,"path":%q}`, serviceName, err.Error(), r.URL.Path)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "upstream_unavailable",
			"service": serviceName,
		})
	}

	return proxy
}

// stripPrefixAndProxy removes a path prefix before proxying.
// Example: /payments/v1/payments -> /v1/payments on the upstream
func stripPrefixAndProxy(prefix string, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		if r2.URL.Path == "" {
			r2.URL.Path = "/"
		}
		proxy.ServeHTTP(w, r2)
	})
}

func loggingMiddleware(cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start)

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
