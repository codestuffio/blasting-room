package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"ok":true}`)) }))
	defer ok.Close()
	if err := healthcheck(ok.URL); err != nil {
		t.Fatalf("healthy server: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer bad.Close()
	if err := healthcheck(bad.URL); err == nil {
		t.Fatal("503 passed the healthcheck")
	}

	bad.Close()
	if err := healthcheck(bad.URL); err == nil {
		t.Fatal("a closed port passed the healthcheck")
	}
}
