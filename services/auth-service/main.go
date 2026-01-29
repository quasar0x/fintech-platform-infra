package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

type appState struct {
	db         *sql.DB
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	env        string
	appName    string
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type customClaims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

func main() {
	port := getenv("PORT", "8081")
	appName := getenv("APP_NAME", "auth-service")
	env := getenv("ENVIRONMENT", "dev")

	issuer := getenv("JWT_ISSUER", "fintech-auth")
	audience := getenv("JWT_AUDIENCE", "fintech-platform")
	accessTTL := mustDurationSeconds(getenv("ACCESS_TOKEN_TTL_SECONDS", "900"))
	refreshTTL := mustDurationSeconds(getenv("REFRESH_TOKEN_TTL_SECONDS", "604800"))

	// --- Load JWT keys ---
	// Private key: prefer file path (K8s secret mount) fallback to env var for local/dev
	privateKey, err := loadRSAPrivateKey()
	if err != nil {
		log.Fatalf("JWT_PRIVATE_KEY invalid/missing: %v", err)
	}

	// Public key: env var (safe to keep in values.yaml as it's public)
	publicKey, err := parseRSAPublicKeyFromPEM(os.Getenv("JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatalf("JWT_PUBLIC_KEY invalid/missing: %v", err)
	}

	// --- DB ---
	dsn, err := buildPostgresDSNFromEnv()
	if err != nil {
		log.Fatalf("DB env invalid/missing: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("DB ping failed (startup): %v", err)
	}

	st := &appState{
		db:         db,
		privateKey: privateKey,
		publicKey:  publicKey,
		env:        env,
		appName:    appName,
		issuer:     issuer,
		audience:   audience,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if st.privateKey == nil || st.publicKey == nil {
			http.Error(w, "jwt keys not loaded", http.StatusServiceUnavailable)
			return
		}
		if err := st.db.PingContext(ctx); err != nil {
			http.Error(w, "db not ready: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeText(w, http.StatusOK, "ready")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, http.StatusOK, fmt.Sprintf("%s running (%s)", st.appName, st.env))
	})

	mux.HandleFunc("/register", st.handleRegister)
	mux.HandleFunc("/login", st.handleLogin)
	mux.HandleFunc("/refresh", st.handleRefresh)
	mux.HandleFunc("/logout", st.handleLogout)
	mux.HandleFunc("/me", st.handleMe)

	addr := ":" + port
	log.Printf("starting %s on %s (env=%s)", st.appName, addr, st.env)
	srv := &http.Server{Addr: addr, Handler: mux}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (st *appState) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	pass := strings.TrimSpace(req.Password)
	if email == "" || pass == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}
	if len(pass) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var userID string
	err = st.db.QueryRowContext(ctx,
		`INSERT INTO users(email, password_hash) VALUES ($1,$2) RETURNING id::text`,
		email, string(hash),
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			http.Error(w, "email already exists", http.StatusConflict)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	_, _ = st.db.ExecContext(ctx, `
		INSERT INTO user_roles(user_id, role_id)
		SELECT $1::uuid, r.id FROM roles r WHERE r.name='user'
		ON CONFLICT DO NOTHING
	`, userID)

	roles := []string{"user"}
	resp, err := st.issueTokens(ctx, userID, roles)
	if err != nil {
		http.Error(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (st *appState) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	pass := strings.TrimSpace(req.Password)
	if email == "" || pass == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var userID, passwordHash, status string
	err := st.db.QueryRowContext(ctx,
		`SELECT id::text, password_hash, status FROM users WHERE email=$1`,
		email,
	).Scan(&userID, &passwordHash, &status)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if status != "active" {
		http.Error(w, "user not active", http.StatusForbidden)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(pass)) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	roles, err := st.getUserRoles(ctx, userID)
	if err != nil {
		http.Error(w, "failed to load roles", http.StatusInternalServerError)
		return
	}

	resp, err := st.issueTokens(ctx, userID, roles)
	if err != nil {
		http.Error(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (st *appState) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(req.RefreshToken)
	if raw == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := st.validateRefreshToken(ctx, raw)
	if !ok {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	roles, err := st.getUserRoles(ctx, userID)
	if err != nil {
		http.Error(w, "failed to load roles", http.StatusInternalServerError)
		return
	}

	if err := st.revokeRefreshToken(ctx, raw); err != nil {
		http.Error(w, "failed to revoke refresh", http.StatusInternalServerError)
		return
	}

	resp, err := st.issueTokens(ctx, userID, roles)
	if err != nil {
		http.Error(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (st *appState) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(req.RefreshToken)
	if raw == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_ = st.revokeRefreshToken(ctx, raw)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (st *appState) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokenStr := bearerToken(r.Header.Get("Authorization"))
	if tokenStr == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}

	claims := &customClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected alg: %s", token.Method.Alg())
		}
		return st.publicKey, nil
	}, jwt.WithAudience(st.audience), jwt.WithIssuer(st.issuer))
	if err != nil || !parsed.Valid {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sub":   claims.Subject,
		"roles": claims.Roles,
		"iss":   claims.Issuer,
		"aud":   claims.Audience,
		"exp":   claims.ExpiresAt.Time.Unix(),
	})
}

func (st *appState) issueTokens(ctx context.Context, userID string, roles []string) (*tokenResponse, error) {
	now := time.Now()

	accessClaims := &customClaims{
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    st.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{st.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(st.accessTTL)),
		},
	}

	access := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessSigned, err := access.SignedString(st.privateKey)
	if err != nil {
		return nil, err
	}

	refreshRaw, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshHash := hashToken(refreshRaw)
	refreshExp := now.Add(st.refreshTTL)

	_, err = st.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens(user_id, token_hash, expires_at) VALUES ($1::uuid,$2,$3)`,
		userID, refreshHash, refreshExp,
	)
	if err != nil {
		return nil, err
	}

	return &tokenResponse{
		AccessToken:  accessSigned,
		RefreshToken: refreshRaw,
		TokenType:    "Bearer",
		ExpiresIn:    int64(st.accessTTL.Seconds()),
	}, nil
}

func (st *appState) getUserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := st.db.QueryContext(ctx, `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1::uuid
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	if len(roles) == 0 {
		roles = []string{"user"}
	}
	return roles, nil
}

func (st *appState) validateRefreshToken(ctx context.Context, raw string) (string, bool) {
	h := hashToken(raw)
	var uid string
	var expires time.Time
	var revoked bool
	err := st.db.QueryRowContext(ctx, `
		SELECT user_id::text, expires_at, revoked
		FROM refresh_tokens
		WHERE token_hash = $1
	`, h).Scan(&uid, &expires, &revoked)
	if err != nil {
		return "", false
	}
	if revoked || time.Now().After(expires) {
		return "", false
	}
	return uid, true
}

func (st *appState) revokeRefreshToken(ctx context.Context, raw string) error {
	h := hashToken(raw)
	_, err := st.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked=true WHERE token_hash=$1`, h)
	return err
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func mustDurationSeconds(s string) time.Duration {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(n) * time.Second
}

func buildPostgresDSNFromEnv() (string, error) {
	host := os.Getenv("DB_HOST")
	port := getenv("DB_PORT", "5432")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	ssl := getenv("DB_SSLMODE", "require")

	missing := []string{}
	if host == "" {
		missing = append(missing, "DB_HOST")
	}
	if name == "" {
		missing = append(missing, "DB_NAME")
	}
	if user == "" {
		missing = append(missing, "DB_USER")
	}
	if pass == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing env vars: %s", strings.Join(missing, ", "))
	}

	return fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		host, port, name, user, pass, ssl,
	), nil
}

// --- JWT key loading helpers ---

func loadRSAPrivateKey() (*rsa.PrivateKey, error) {
	// Preferred: file path (Kubernetes secret mount).
	// If JWT_PRIVATE_KEY_PATH is not set, we default to the common mount path.
	path := strings.TrimSpace(os.Getenv("JWT_PRIVATE_KEY_PATH"))
	if path == "" {
		path = "/var/run/secrets/jwt/jwt_private.pem"
	}

	// Try reading from file first
	b, err := os.ReadFile(path)
	if err == nil {
		if strings.TrimSpace(string(b)) == "" {
			// File exists but empty -> treat as invalid and fallback to env
			log.Printf("warning: JWT private key file exists but is empty: %s (will fallback to env JWT_PRIVATE_KEY)", path)
		} else {
			log.Printf("loaded JWT private key from file: %s", path)
			return parseRSAPrivateKeyFromPEM(string(b))
		}
	} else {
		log.Printf("warning: unable to read JWT private key file %s: %v (will fallback to env JWT_PRIVATE_KEY)", path, err)
	}

	// Fallback: env var (useful for local dev)
	pemStr := os.Getenv("JWT_PRIVATE_KEY")
	if strings.TrimSpace(pemStr) == "" {
		return nil, fmt.Errorf("private key not found (file=%s unreadable/empty and JWT_PRIVATE_KEY env is empty)", path)
	}
	log.Printf("loaded JWT private key from env JWT_PRIVATE_KEY (fallback)")
	return parseRSAPrivateKeyFromPEM(pemStr)
}

func parseRSAPrivateKeyFromPEM(pemStr string) (*rsa.PrivateKey, error) {
	pemStr = strings.TrimSpace(pemStr)
	if pemStr == "" {
		return nil, errors.New("empty")
	}
	return jwt.ParseRSAPrivateKeyFromPEM([]byte(pemStr))
}

func parseRSAPublicKeyFromPEM(pemStr string) (*rsa.PublicKey, error) {
	pemStr = strings.TrimSpace(pemStr)
	if pemStr == "" {
		return nil, errors.New("empty")
	}
	return jwt.ParseRSAPublicKeyFromPEM([]byte(pemStr))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeText(w http.ResponseWriter, code int, s string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(s))
}

func bearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
