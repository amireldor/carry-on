package main

import (
	"os"
	"testing"
)

func TestNormalizeTarget(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"5000", "http://localhost:5000"},
		{"localhost:5000", "http://localhost:5000"},
		{"http://localhost:5000", "http://localhost:5000"},
		{"https://api.example.com", "https://api.example.com"},
		{"api.example.com:8080", "http://api.example.com:8080"},
	}
	for _, c := range cases {
		if got := normalizeTarget(c.in); got != c.want {
			t.Errorf("normalizeTarget(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/api", "/api"},
		{"/api/", "/api"},
		{"api", "/api"},
		{"", "/"},
		{"/", "/"},
	}
	for _, c := range cases {
		if got := cleanPath(c.in); got != c.want {
			t.Errorf("cleanPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRouteSpec(t *testing.T) {
	r, err := parseRouteSpec("/api@5000", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != "/api" {
		t.Errorf("Path = %q, want /api", r.Path)
	}
	if r.Target != "http://localhost:5000" {
		t.Errorf("Target = %q, want http://localhost:5000", r.Target)
	}
	if !r.Strip {
		t.Error("Strip should be true")
	}

	// full URL target
	r2, err := parseRouteSpec("/ws@http://localhost:5001", false)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Target != "http://localhost:5001" {
		t.Errorf("Target = %q, want http://localhost:5001", r2.Target)
	}
	if r2.Strip {
		t.Error("Strip should be false")
	}

	// invalid
	if _, err := parseRouteSpec("/api-no-at", true); err == nil {
		t.Error("expected error for spec without @")
	}
}

func TestLoadConfigCLIArgs(t *testing.T) {
	cfg, err := loadConfig([]string{"/api@5000", "5713"}, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 1987 {
		t.Errorf("Port = %d, want 1987", cfg.Port)
	}
	if cfg.Fallback != "http://localhost:5713" {
		t.Errorf("Fallback = %q, want http://localhost:5713", cfg.Fallback)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(cfg.Routes))
	}
	if cfg.Routes[0].Path != "/api" {
		t.Errorf("Route[0].Path = %q, want /api", cfg.Routes[0].Path)
	}
	if !cfg.CORS {
		t.Error("CORS should be true by default")
	}
}

func TestLoadConfigNoCORS(t *testing.T) {
	cfg, err := loadConfig([]string{"5000"}, "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CORS {
		t.Error("CORS should be false with --no-cors")
	}
}

func TestLoadConfigNoStrip(t *testing.T) {
	cfg, err := loadConfig([]string{"/api@5000"}, "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Routes[0].Strip {
		t.Error("Strip should be false with --no-strip")
	}
}

func TestLoadConfigPORTEnv(t *testing.T) {
	os.Setenv("PORT", "9000")
	defer os.Unsetenv("PORT")

	cfg, err := loadConfig(nil, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
}

func TestLoadConfigTOML(t *testing.T) {
	f, err := os.CreateTemp("", "carry-on-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString(`
port = 4000
fallback = "localhost:3000"

[[route]]
path = "/api"
target = "localhost:8080"

[[route]]
path = "/ws"
target = "localhost:8081"
strip = false
`)
	f.Close()

	cfg, err := loadConfig(nil, f.Name(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 4000 {
		t.Errorf("Port = %d, want 4000", cfg.Port)
	}
	if cfg.Fallback != "http://localhost:3000" {
		t.Errorf("Fallback = %q", cfg.Fallback)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("len(Routes) = %d, want 2", len(cfg.Routes))
	}
	if !cfg.Routes[0].Strip {
		t.Error("Routes[0].Strip should default to true")
	}
	if cfg.Routes[1].Strip {
		t.Error("Routes[1].Strip should be false")
	}
}

func TestLoadConfigCLIOverridesFile(t *testing.T) {
	f, err := os.CreateTemp("", "carry-on-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(`fallback = "localhost:3000"` + "\n[[route]]\npath = \"/file\"\ntarget = \"localhost:9999\"\n")
	f.Close()

	// CLI args should replace file routes entirely
	cfg, err := loadConfig([]string{"/api@5000"}, f.Name(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Path != "/api" {
		t.Errorf("CLI routes should override file routes, got %+v", cfg.Routes)
	}
	if cfg.Fallback != "" {
		t.Errorf("CLI args with no fallback should clear file fallback, got %q", cfg.Fallback)
	}
}
