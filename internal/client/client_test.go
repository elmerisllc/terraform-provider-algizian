package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func noSleep(c *Client) { c.Sleep = func(context.Context, time.Duration) error { return nil } }

func TestProblemAnswerBecomesAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(400)
		w.Write([]byte(`{"title":"Bad Request","status":400,"detail":"The slug must be 4 to 40 characters.","field":"slug"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "ag_x", "test")
	err := c.Do(context.Background(), "POST", "/tenants", map[string]any{"slug": "x"}, nil)
	var e *APIError
	if err == nil || !IsStatus(err, 400) {
		t.Fatalf("want a 400 APIError, got %v", err)
	}
	e = err.(*APIError)
	if e.Field != "slug" || e.Detail == "" {
		t.Fatalf("problem fields not parsed: %+v", e)
	}
}

func TestBearerTokenAndNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ag_secret" || r.Header.Get("User-Agent") != "ua" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	err := New(srv.URL, "ag_secret", "ua").Do(context.Background(), "GET", "/tenants/x", nil, nil)
	if !IsNotFound(err) {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestRateLimitIsRetried(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(429)
			return
		}
		w.Write([]byte(`{"scope":"platform"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "t", "")
	noSleep(c)
	me, err := c.Me(context.Background())
	if err != nil || me.Scope != "platform" || n != 3 {
		t.Fatalf("me=%+v err=%v calls=%d", me, err, n)
	}
}

func TestPostIsNotRetriedOnServerError(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := New(srv.URL, "t", "")
	noSleep(c)
	if err := c.Do(context.Background(), "POST", "/tenants", map[string]any{}, nil); !IsStatus(err, 503) || n != 1 {
		t.Fatalf("err=%v calls=%d", err, n)
	}
}
