package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInit()
		return
	}

	var configFile string
	var noStrip, noCORS bool
	flag.StringVar(&configFile, "config", "", "path to config file")
	flag.StringVar(&configFile, "c", "", "path to config file")
	flag.BoolVar(&noStrip, "no-strip", false, "disable path prefix stripping")
	flag.BoolVar(&noCORS, "no-cors", false, "disable CORS header injection")
	flag.Parse()

	cfg, err := loadConfig(flag.Args(), configFile, noStrip, noCORS)
	if err != nil {
		log.Fatalf("carry-on: %v", err)
	}

	printBanner(cfg)

	addr := ":" + strconv.Itoa(cfg.Port)
	if err := http.ListenAndServe(addr, newHandler(cfg)); err != nil {
		log.Fatalf("carry-on: %v", err)
	}
}

func printBanner(cfg *Config) {
	fmt.Printf("carry-on  ▶  :%d\n", cfg.Port)
	for _, r := range cfg.Routes {
		suffix := ""
		if r.Strip {
			suffix = "  (strip)"
		}
		fmt.Printf("  %-14s →  %s%s\n", r.Path, displayTarget(r.Target), suffix)
	}
	if cfg.Fallback != "" {
		fmt.Printf("  %-14s →  %s  (fallback)\n", "*", displayTarget(cfg.Fallback))
	}
}

func displayTarget(t string) string {
	t = strings.TrimPrefix(t, "http://")
	t = strings.TrimPrefix(t, "https://")
	return t
}

const initTemplate = `# carry-on.toml
# Proxy listening port (default: 1987, override with PORT env var)
# port = 1987

# Fallback: catch-all target for unmatched paths
fallback = "localhost:3000"

# Route: forward /api/* to port 8080, stripping the /api prefix
[[route]]
path = "/api"
target = "localhost:8080"
# strip = true  # default: strips the matched prefix before forwarding

# Route: forward /ws/* without stripping
# [[route]]
# path = "/ws"
# target = "localhost:8081"
# strip = false
`

func runInit() {
	const filename = "carry-on.toml"
	if _, err := os.Stat(filename); err == nil {
		fmt.Println("carry-on.toml already exists")
		return
	}
	if err := os.WriteFile(filename, []byte(initTemplate), 0644); err != nil {
		log.Fatalf("carry-on: %v", err)
	}
	fmt.Println("Created carry-on.toml")
}
