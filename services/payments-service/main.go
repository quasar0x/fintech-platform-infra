package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type appState struct {
	dbReady bool
	db      *sql.DB
}

type createPaymentRequest struct {
	UserID   string `json:"user_id"`
	Amount   int64  `json:"amount"`   // store in minor units (e.g., kobo/cents)
	Currency string `json:"currency"` // "NGN", "USD"
	Ref      string `json:"ref"`      // client reference / idempotency-ish
}

type payment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	Ref       string    `json:"ref"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	port := getenv("PORT", "8083")
	app := getenv("APP_NAME", "payments-service")
	env := getenv("ENVIRONMENT", "dev")

	dsn, err := buildPostgresDSNFromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	st := &appState{db: db, dbReady: false}

	// Background readiness ping
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			st.dbReady = (err == nil)
		}
	}()

	// Ensure schema exists (safe / idempotent)
	if err := ensureSchema(db); err != nil {
		log.Fatalf("schema init failed: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !st.dbReady {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}
		writeText(w, http.StatusOK, "ready")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, http.StatusOK, fmt.Sprintf("%s running (%s)", app, env))
	})

	// API
	mux.HandleFunc("/v1/payments", st.handlePayments)     // POST create, GET list (optional query)
	mux.HandleFunc("/v1/payments/", st.handlePaymentByID) // GET /v1/payments/{id}

	addr := ":" + port
	log.Printf("starting %s on %s (env=%s)", app, addr, env)

	srv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(app, env, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (st *appState) handlePayments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		st.createPayment(w, r)
	case http.MethodGet:
		st.listPayments(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (st *appState) createPayment(w http.ResponseWriter, r *http.Request) {
	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	req.Ref = strings.TrimSpace(req.Ref)

	if req.UserID == "" || req.Amount <= 0 || req.Currency == "" || req.Ref == "" {
		http.Error(w, "user_id, amount (>0), currency, ref are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var p payment
	err := st.db.QueryRowContext(ctx, `
		INSERT INTO payments(user_id, amount, currency, status, ref)
		VALUES ($1, $2, $3, 'created', $4)
		RETURNING id::text, user_id, amount, currency, status, ref, created_at
	`, req.UserID, req.Amount, req.Currency, req.Ref).Scan(
		&p.ID, &p.UserID, &p.Amount, &p.Currency, &p.Status, &p.Ref, &p.CreatedAt,
	)
	if err != nil {
		// unique ref per user (see schema) treat duplicates as conflict
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			http.Error(w, "duplicate ref", http.StatusConflict)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func (st *appState) listPayments(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var rows *sql.Rows
	var err error

	if userID == "" {
		rows, err = st.db.QueryContext(ctx, `
			SELECT id::text, user_id, amount, currency, status, ref, created_at
			FROM payments
			ORDER BY created_at DESC
			LIMIT 50
		`)
	} else {
		rows, err = st.db.QueryContext(ctx, `
			SELECT id::text, user_id, amount, currency, status, ref, created_at
			FROM payments
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT 50
		`, userID)
	}

	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []payment{}
	for rows.Next() {
		var p payment
		if err := rows.Scan(&p.ID, &p.UserID, &p.Amount, &p.Currency, &p.Status, &p.Ref, &p.CreatedAt); err != nil {
			http.Error(w, "db scan error", http.StatusInternalServerError)
			return
		}
		out = append(out, p)
	}

	writeJSON(w, http.StatusOK, out)
}

func (st *appState) handlePaymentByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// path: /v1/payments/{id}
	id := strings.TrimPrefix(r.URL.Path, "/v1/payments/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var p payment
	err := st.db.QueryRowContext(ctx, `
		SELECT id::text, user_id, amount, currency, status, ref, created_at
		FROM payments
		WHERE id = $1::uuid
	`, id).Scan(&p.ID, &p.UserID, &p.Amount, &p.Currency, &p.Status, &p.Ref, &p.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func ensureSchema(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// keep it simple: one table
	_, err := db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS payments (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id text NOT NULL,
			amount bigint NOT NULL,
			currency text NOT NULL,
			status text NOT NULL,
			ref text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		);

		-- prevent duplicate "ref" per user (acts like lightweight idempotency)
		CREATE UNIQUE INDEX IF NOT EXISTS payments_user_ref_uq ON payments(user_id, ref);
	`)
	return err
}

func buildPostgresDSNFromEnv() (string, error) {
	host := os.Getenv("DB_HOST")
	port := getenv("DB_PORT", "5432")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	ssl := getenv("DB_SSLMODE", "require")

	if host == "" || name == "" || user == "" || pass == "" {
		return "", fmt.Errorf("missing required DB env vars (DB_HOST/DB_NAME/DB_USER/DB_PASSWORD)")
	}

	return fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		host, port, name, user, pass, ssl,
	), nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loggingMiddleware(app, env string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start)
		log.Printf(`{"app":"%s","env":"%s","method":"%s","path":"%s","duration_ms":%d}`,
			app, env, r.Method, r.URL.Path, d.Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeText(w http.ResponseWriter, status int, s string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(s))
}
