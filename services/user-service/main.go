package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type appState struct {
	dbReady bool
	db      *sql.DB
}

func main() {
	port := getenv("PORT", "8082")
	app := getenv("APP_NAME", "user-service")
	env := getenv("ENVIRONMENT", "dev")

	// ---- OpenTelemetry (minimal init) ----
	ctx := context.Background()
	tcfg := loadTelemetryConfig()
	shutdown, err := initTracer(ctx, tcfg)
	if err != nil {
		log.Fatalf("otel init error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	// -------------------------------------

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

	state := &appState{db: db, dbReady: false}

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()

		for range t.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			state.dbReady = (err == nil)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !state.dbReady {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("%s running (%s)", app, env)))
	})

	addr := ":" + port
	log.Printf("starting %s on %s (env=%s)", app, addr, env)

	srv := &http.Server{
		Addr:    addr,
		Handler: otelhttp.NewHandler(mux, "user-service"),
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
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
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
