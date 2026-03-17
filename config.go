package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Route struct {
	Path   string
	Target string // normalized "http://host:port"
	Strip  bool
}

type Config struct {
	Port     int
	Fallback string // normalized URL, empty if none
	CORS     bool
	Routes   []Route
}

// toml-mapped types with optional fields

type tomlRoute struct {
	Path   string `toml:"path"`
	Target string `toml:"target"`
	Strip  *bool  `toml:"strip"`
}

type tomlConfig struct {
	Port     int         `toml:"port"`
	Fallback string      `toml:"fallback"`
	CORS     *bool       `toml:"cors"`
	Routes   []tomlRoute `toml:"route"`
}

func loadConfig(args []string, configFile string, noStrip bool, noCORS bool) (*Config, error) {
	cfg := &Config{
		Port: 1987,
		CORS: true,
	}

	// Load config file (explicit path or auto-detect carry-on.toml)
	var fileCfg tomlConfig
	loaded := false
	if configFile != "" {
		if _, err := toml.DecodeFile(configFile, &fileCfg); err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		loaded = true
	} else if _, err := os.Stat("carry-on.toml"); err == nil {
		if _, err := toml.DecodeFile("carry-on.toml", &fileCfg); err != nil {
			return nil, fmt.Errorf("reading carry-on.toml: %w", err)
		}
		loaded = true
	}

	if loaded {
		if fileCfg.Port > 0 {
			cfg.Port = fileCfg.Port
		}
		if fileCfg.Fallback != "" {
			cfg.Fallback = normalizeTarget(fileCfg.Fallback)
		}
		if fileCfg.CORS != nil {
			cfg.CORS = *fileCfg.CORS
		}
		for _, r := range fileCfg.Routes {
			strip := true
			if r.Strip != nil {
				strip = *r.Strip
			}
			cfg.Routes = append(cfg.Routes, Route{
				Path:   cleanPath(r.Path),
				Target: normalizeTarget(r.Target),
				Strip:  strip,
			})
		}
	}

	// PORT env var always overrides
	if port := os.Getenv("PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %s", port)
		}
		cfg.Port = p
	}

	// CLI args override routes entirely
	if len(args) > 0 {
		cfg.Routes = nil
		cfg.Fallback = ""
		for _, arg := range args {
			// bare number = fallback port
			if _, err := strconv.Atoi(arg); err == nil {
				cfg.Fallback = normalizeTarget(arg)
				continue
			}
			route, err := parseRouteSpec(arg, !noStrip)
			if err != nil {
				return nil, err
			}
			cfg.Routes = append(cfg.Routes, route)
		}
	}

	if noCORS {
		cfg.CORS = false
	}

	return cfg, nil
}

func parseRouteSpec(spec string, strip bool) (Route, error) {
	idx := strings.LastIndex(spec, "@")
	if idx < 0 {
		return Route{}, fmt.Errorf("invalid route %q: expected /path@target", spec)
	}
	return Route{
		Path:   cleanPath(spec[:idx]),
		Target: normalizeTarget(spec[idx+1:]),
		Strip:  strip,
	}, nil
}

func normalizeTarget(t string) string {
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		return t
	}
	if _, err := strconv.Atoi(t); err == nil {
		return "http://localhost:" + t
	}
	return "http://" + t
}

func cleanPath(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" || p[0] != '/' {
		p = "/" + p
	}
	return p
}
