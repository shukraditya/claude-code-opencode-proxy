package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/shukraditya/claude-code-proxy/config"
	"github.com/shukraditya/claude-code-proxy/proxy"
)

func main() {
	envPath := config.DefaultEnvPath()
	cfgMgr := config.NewManager(envPath)
	cfg := cfgMgr.Get()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	handler := proxy.NewHandler(cfgMgr)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting Claude Code Proxy on %s", addr)
	log.Printf("Model: %s", cfg.BigModel)
	log.Printf("OpenAI Base URL: %s", cfg.OpenAIBaseURL)
	log.Printf("Watching %s for changes", envPath)

	if cfg.AnthropicAPIKey != "" {
		log.Printf("API key validation enabled")
	} else {
		log.Printf("API key validation disabled")
	}

	log.Printf("")
	log.Printf("To use with Claude Code, set:")
	log.Printf("  ANTHROPIC_BASE_URL=http://%s ANTHROPIC_API_KEY=any-value claude", addr)
	log.Printf("")

	stop := make(chan struct{})
	changes := cfgMgr.Watch(stop)
	go func() {
		for range changes {
			c := cfgMgr.Get()
			log.Printf("Config reloaded — model: %s, base URL: %s", c.BigModel, c.OpenAIBaseURL)
		}
	}()

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
