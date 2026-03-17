package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeHandler is a test helper that builds a handler from a simple config.
func makeHandler(cors bool, routes []Route, fallback string) *handler {
	return newHandler(&Config{
		Port:     1987,
		CORS:     cors,
		Routes:   routes,
		Fallback: fallback,
	})
}

// backendEcho starts a test backend that echoes the request path in the response body.
func backendEcho(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Path))
	}))
}

func TestCORSPreflight(t *testing.T) {
	h := makeHandler(true, nil, "")

	req := httptest.NewRequest(http.MethodOptions, "/anything", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods should be set")
	}
}

func TestCORSPreflightDisabled(t *testing.T) {
	h := makeHandler(false, nil, "")

	req := httptest.NewRequest(http.MethodOptions, "/anything", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// With CORS disabled and no routes, should 502 (not handle as preflight)
	if rec.Code == http.StatusNoContent {
		t.Error("should not handle OPTIONS as preflight when CORS is disabled")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not be set when CORS is disabled")
	}
}

func TestNoRouteMatch(t *testing.T) {
	h := makeHandler(true, nil, "") // no routes, no fallback

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestRouteMatchWithStrip(t *testing.T) {
	backend := backendEcho(t)
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: backend.URL, Strip: true},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "/users" {
		t.Errorf("backend received path %q, want /users", body)
	}
}

func TestRouteMatchWithoutStrip(t *testing.T) {
	backend := backendEcho(t)
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: backend.URL, Strip: false},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "/api/users" {
		t.Errorf("backend received path %q, want /api/users", body)
	}
}

func TestStripPrefixExactMatch(t *testing.T) {
	backend := backendEcho(t)
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: backend.URL, Strip: true},
	}, "")

	// Exact match with no trailing slash
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if body != "/" {
		t.Errorf("backend received path %q, want /", body)
	}
}

func TestFallback(t *testing.T) {
	backend := backendEcho(t)
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: "http://localhost:1", Strip: true}, // unreachable, should not match
	}, backend.URL)

	req := httptest.NewRequest(http.MethodGet, "/other/path", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "/other/path" {
		t.Errorf("backend received path %q, want /other/path", body)
	}
}

func TestLongestPrefixMatch(t *testing.T) {
	short := backendEcho(t)
	defer short.Close()
	long := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("long"))
	}))
	defer long.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: short.URL, Strip: false},
		{Path: "/api/v2", Target: long.URL, Strip: false},
	}, "")

	// /api/v2/users should match /api/v2, not /api
	req := httptest.NewRequest(http.MethodGet, "/api/v2/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Body.String() != "long" {
		t.Errorf("expected long backend to handle /api/v2/users")
	}

	// /api/v1 should match /api
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Body.String() != "/api/v1/users" {
		t.Errorf("expected short backend, got %q", rec2.Body.String())
	}
}

func TestNoPrefixPartialMatch(t *testing.T) {
	// /apifoo should NOT match a route for /api
	backend := backendEcho(t)
	defer backend.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fallback"))
	}))
	defer fallback.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: backend.URL, Strip: false},
	}, fallback.URL)

	req := httptest.NewRequest(http.MethodGet, "/apifoo/bar", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Body.String() != "fallback" {
		t.Errorf("/apifoo/bar should not match /api route, got %q", rec.Body.String())
	}
}

func TestCORSHeadersInjected(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// backend sets a conflicting CORS header
		w.Header().Set("Access-Control-Allow-Origin", "https://production.com")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/api", Target: backend.URL, Strip: true},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// carry-on should override the backend's restrictive CORS header
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want * (should override backend header)", got)
	}
}

func TestCORSHeadersNotInjectedWhenDisabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	h := makeHandler(false, []Route{
		{Path: "/api", Target: backend.URL, Strip: true},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty when CORS disabled", got)
	}
}

func TestStripPrefix(t *testing.T) {
	cases := []struct {
		path, prefix, want string
	}{
		{"/api/users", "/api", "/users"},
		{"/api", "/api", "/"},
		{"/api/", "/api", "/"},
		{"/", "/", "/"},
	}
	for _, c := range cases {
		got := stripPrefix(c.path, c.prefix)
		if got != c.want {
			t.Errorf("stripPrefix(%q, %q) = %q, want %q", c.path, c.prefix, got, c.want)
		}
	}
}

func TestIsWebSocket(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if isWebSocket(req) {
		t.Error("should not be websocket without Upgrade header")
	}

	req.Header.Set("Upgrade", "websocket")
	if !isWebSocket(req) {
		t.Error("should be websocket with Upgrade: websocket")
	}

	req.Header.Set("Upgrade", "WebSocket") // case-insensitive
	if !isWebSocket(req) {
		t.Error("Upgrade header check should be case-insensitive")
	}
}

func TestWebSocketProxy(t *testing.T) {
	// Real WebSocket backend using raw HTTP upgrade
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "expected websocket", http.StatusBadRequest)
			return
		}
		// Echo back the path to verify routing/stripping worked
		hijacker := w.(http.Hijacker)
		conn, _, _ := hijacker.Hijack()
		defer conn.Close()
		// Send minimal HTTP 101 + custom header with the path
		resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nX-Received-Path: " + r.URL.Path + "\r\n\r\n"
		conn.Write([]byte(resp))
	}))
	defer backend.Close()

	h := makeHandler(true, []Route{
		{Path: "/ws", Target: backend.URL, Strip: true},
	}, "")

	// Use httptest.Server to get a real net.Conn (httptest.NewRecorder doesn't support Hijack)
	proxy := httptest.NewServer(h)
	defer proxy.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send WebSocket upgrade request
	conn.Write([]byte("GET /ws/chat HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	response := string(buf[:n])

	if !strings.Contains(response, "101") {
		t.Errorf("expected 101 response, got: %q", response)
	}
	// Backend should have received path /chat (prefix stripped)
	if !strings.Contains(response, "X-Received-Path: /chat") {
		t.Errorf("backend should receive stripped path /chat, response: %q", response)
	}
}
