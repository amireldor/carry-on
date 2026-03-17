package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
)

var corsHeaders = [][2]string{
	{"Access-Control-Allow-Origin", "*"},
	{"Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS"},
	{"Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With"},
	{"Access-Control-Allow-Credentials", "true"},
}

type routeEntry struct {
	route Route
	proxy *httputil.ReverseProxy
}

type handler struct {
	cfg    *Config
	routes []routeEntry // sorted longest-prefix first; fallback (path="") is last
}

func newHandler(cfg *Config) *handler {
	h := &handler{cfg: cfg}

	all := append([]Route{}, cfg.Routes...)
	if cfg.Fallback != "" {
		all = append(all, Route{Path: "", Target: cfg.Fallback, Strip: false})
	}

	for _, r := range all {
		r := r // capture
		target, _ := url.Parse(r.Target)

		p := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.Host = target.Host
				if r.Strip && r.Path != "" {
					req.URL.Path = stripPrefix(req.URL.Path, r.Path)
					if req.URL.RawPath != "" {
						req.URL.RawPath = stripPrefix(req.URL.RawPath, r.Path)
					}
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				if cfg.CORS {
					setCORS(resp.Header)
				}
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
				log.Printf("proxy error → %s: %v", r.Target, err)
				http.Error(w, "backend unavailable", http.StatusBadGateway)
			},
			FlushInterval: -1,
		}
		h.routes = append(h.routes, routeEntry{route: r, proxy: p})
	}

	sort.Slice(h.routes, func(i, j int) bool {
		return len(h.routes[i].route.Path) > len(h.routes[j].route.Path)
	})

	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions && h.cfg.CORS {
		setCORS(w.Header())
		w.WriteHeader(http.StatusNoContent)
		return
	}

	entry := h.match(r.URL.Path)
	if entry == nil {
		http.Error(w, "no route matched", http.StatusBadGateway)
		return
	}

	if isWebSocket(r) {
		h.proxyWebSocket(w, r, entry.route)
		return
	}

	entry.proxy.ServeHTTP(w, r)
}

func (h *handler) match(path string) *routeEntry {
	for i := range h.routes {
		e := &h.routes[i]
		if e.route.Path == "" {
			return e // fallback matches everything
		}
		if strings.HasPrefix(path, e.route.Path) {
			rest := path[len(e.route.Path):]
			if rest == "" || rest[0] == '/' {
				return e
			}
		}
	}
	return nil
}

func (h *handler) proxyWebSocket(w http.ResponseWriter, r *http.Request, route Route) {
	target, _ := url.Parse(route.Target)

	if route.Strip && route.Path != "" {
		r.URL.Path = stripPrefix(r.URL.Path, route.Path)
		r.URL.RawPath = ""
	}
	r.URL.Host = target.Host
	r.URL.Scheme = target.Scheme
	r.Host = target.Host
	r.RequestURI = ""

	backendConn, err := net.Dial("tcp", target.Host)
	if err != nil {
		log.Printf("websocket error → %s: %v", route.Target, err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return
	}
	clientConn, brw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	if err := r.Write(backendConn); err != nil {
		log.Printf("websocket handshake error: %v", err)
		return
	}

	errc := make(chan error, 2)
	go func() { _, err := io.Copy(backendConn, brw); errc <- err }()
	go func() { _, err := io.Copy(clientConn, backendConn); errc <- err }()
	<-errc
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func stripPrefix(path, prefix string) string {
	stripped := strings.TrimPrefix(path, prefix)
	if stripped == "" || stripped[0] != '/' {
		stripped = "/" + stripped
	}
	return stripped
}

func setCORS(h http.Header) {
	for _, kv := range corsHeaders {
		h.Set(kv[0], kv[1])
	}
}
