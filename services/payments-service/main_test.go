package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// helper to build the same mux used in main (copy of route wiring)
func newTestMux(st *appState, app, env string) http.Handler {
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
		writeText(w, http.StatusOK, app+" running ("+env+")")
	})

	// keep API routes (we won’t exercise DB in these tests)
	mux.HandleFunc("/v1/payments", st.handlePayments)
	mux.HandleFunc("/v1/payments/", st.handlePaymentByID)

	return mux
}

func TestHealthzReturns200(t *testing.T) {
	st := &appState{dbReady: false}
	h := newTestMux(st, "payments-service", "test")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rr.Body.String())
	}
}

func TestReadyzReturns503WhenDBNotReady(t *testing.T) {
	st := &appState{dbReady: false}
	h := newTestMux(st, "payments-service", "test")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestReadyzReturns200WhenDBReady(t *testing.T) {
	st := &appState{dbReady: true}
	h := newTestMux(st, "payments-service", "test")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ready" {
		t.Fatalf("expected body 'ready', got %q", rr.Body.String())
	}
}
