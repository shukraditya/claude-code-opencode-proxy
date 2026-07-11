package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatAnthropic Format = "anthropic"
)

type ModelInfo struct {
	Model  string
	BaseURL string
	Format Format
}

var ModelRegistry = map[string]ModelInfo{
	"deepseek-v4-flash":  {Model: "deepseek-v4-flash", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"deepseek-v4-pro":    {Model: "deepseek-v4-pro", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"glm-5.2":           {Model: "glm-5.2", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"glm-5.1":           {Model: "glm-5.1", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"kimi-k2.7-code":    {Model: "kimi-k2.7-code", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"kimi-k2.6":         {Model: "kimi-k2.6", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"mimo-v2.5":         {Model: "mimo-v2.5", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"mimo-v2.5-pro":     {Model: "mimo-v2.5-pro", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatOpenAI},
	"minimax-m3":        {Model: "minimax-m3", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
	"minimax-m2.7":      {Model: "minimax-m2.7", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
	"minimax-m2.5":      {Model: "minimax-m2.5", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
	"qwen3.7-max":       {Model: "qwen3.7-max", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
	"qwen3.7-plus":      {Model: "qwen3.7-plus", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
	"qwen3.6-plus":      {Model: "qwen3.6-plus", BaseURL: "https://opencode.ai/zen/go/v1", Format: FormatAnthropic},
}

func LookupModel(name string) (ModelInfo, bool) {
	m, ok := ModelRegistry[name]
	return m, ok
}

type Config struct {
	OpenAIAPIKey    string
	OpenAIBaseURL   string
	AnthropicAPIKey string
	Host            string
	Port            int
	LogLevel        string
	BigModel        string
	ModelInfo       ModelInfo
	RequestTimeout  int
	MaxRetries      int
	CustomHeaders   map[string]string
}

type Manager struct {
	cfg      atomic.Pointer[Config]
	envPath  string
	envMtime time.Time
}

func (c *Config) Validate() error {
	if c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	return nil
}

func (c *Config) AddCustomHeaders(req *http.Request) {
	for k, v := range c.CustomHeaders {
		req.Header.Set(k, v)
	}
}

func NewManager(envPath string) *Manager {
	m := &Manager{envPath: envPath}
	m.load()
	return m
}

func (m *Manager) Get() *Config {
	return m.cfg.Load()
}

func (m *Manager) Watch(stop <-chan struct{}) <-chan struct{} {
	changed := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-stop:
				close(changed)
				return
			case <-time.After(3 * time.Second):
				if m.checkAndReload() {
					select {
					case changed <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	return changed
}

func (m *Manager) checkAndReload() bool {
	info, err := os.Stat(m.envPath)
	if err != nil {
		return false
	}
	if info.ModTime().Equal(m.envMtime) {
		return false
	}
	return m.load()
}

func (m *Manager) load() bool {
	loadEnvFile(m.envPath)

	model := getEnv("BIG_MODEL", "gpt-4o")
	modelInfo, _ := LookupModel(model)
	if modelInfo.Model == "" {
		modelInfo = ModelInfo{
			Model:   model,
			BaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			Format:  FormatOpenAI,
		}
	}
	if explicitURL := os.Getenv("OPENAI_BASE_URL"); explicitURL != "" {
		modelInfo.BaseURL = explicitURL
	}

	cfg := &Config{
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:   modelInfo.BaseURL,
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Host:            getEnv("HOST", "0.0.0.0"),
		Port:            getEnvInt("PORT", 8082),
		LogLevel:        getEnv("LOG_LEVEL", "INFO"),
		BigModel:        modelInfo.Model,
		ModelInfo:       modelInfo,
		RequestTimeout:  getEnvInt("REQUEST_TIMEOUT", 90),
		MaxRetries:      getEnvInt("MAX_RETRIES", 2),
		CustomHeaders:   getCustomHeaders(),
	}

	m.cfg.Store(cfg)

	if info, err := os.Stat(m.envPath); err == nil {
		m.envMtime = info.ModTime()
	}
	return true
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")
		os.Setenv(key, val)
	}
}

func DefaultEnvPath() string {
	wd, _ := os.Getwd()
	if wd == "" {
		return ".env"
	}
	candidates := []string{
		filepath.Join(wd, ".env"),
		filepath.Join(wd, ".env.local"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(wd, ".env")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getCustomHeaders() map[string]string {
	h := map[string]string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CUSTOM_HEADER_") {
			continue
		}
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], "CUSTOM_HEADER_")
		key = strings.ReplaceAll(key, "_", "-")
		h[key] = parts[1]
	}
	return h
}
