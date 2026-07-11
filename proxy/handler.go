package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/shukraditya/claude-code-proxy/client"
	"github.com/shukraditya/claude-code-proxy/config"
	"github.com/shukraditya/claude-code-proxy/conversion"
	"github.com/shukraditya/claude-code-proxy/models"
)

type Handler struct {
	cfgMgr *config.Manager
}

func NewHandler(cfgMgr *config.Manager) *Handler {
	return &Handler{cfgMgr: cfgMgr}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/messages":
		if r.Method == http.MethodPost {
			h.handleMessages(w, r)
			return
		}
	case "/v1/messages/count_tokens":
		if r.Method == http.MethodPost {
			h.handleCountTokens(w, r)
			return
		}
	case "/health":
		h.handleHealth(w, r)
		return
	case "/":
		h.handleRoot(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfgMgr.Get()

	if !h.validateAPIKey(cfg, r) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, http.StatusUnauthorized)
		return
	}

	var claudeReq models.ClaudeMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&claudeReq); err != nil {
		http.Error(w, fmt.Sprintf(`{"type":"error","error":{"type":"invalid_request_error","message":"invalid JSON: %s"}}`, err.Error()), http.StatusBadRequest)
		return
	}

	conv := conversion.NewRequestConverter(cfg)
	openaiReq, mappedModel, err := conv.Convert(&claudeReq)
	if err != nil {
		log.Printf("conversion error: %v", err)
		http.Error(w, fmt.Sprintf(`{"type":"error","error":{"type":"invalid_request_error","message":"conversion failed: %s"}}`, err.Error()), http.StatusBadRequest)
		return
	}

	cl := client.NewOpenAIClient(cfg)
	respConv := conversion.NewResponseConverter(mappedModel)

	log.Printf("→ %s | model=%s → %s | stream=%v", r.RemoteAddr, claudeReq.Model, mappedModel, claudeReq.Stream)

	if claudeReq.Stream {
		h.handleStreaming(r.Context(), w, cl, respConv, openaiReq)
	} else {
		h.handleNonStreaming(r.Context(), w, cl, respConv, openaiReq)
	}
}

func (h *Handler) handleNonStreaming(ctx context.Context, w http.ResponseWriter, cl *client.OpenAIClient, respConv *conversion.ResponseConverter, openaiReq *models.OpenAIRequest) {
	openaiResp, err := cl.CreateChatCompletion(ctx, openaiReq)
	if err != nil {
		log.Printf("OpenAI API error: %v", err)
		http.Error(w, fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":"%s"}}`, err.Error()), http.StatusBadGateway)
		return
	}

	claudeResp := respConv.ConvertNonStreaming(openaiResp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(claudeResp)
}

func (h *Handler) handleStreaming(ctx context.Context, w http.ResponseWriter, cl *client.OpenAIClient, respConv *conversion.ResponseConverter, openaiReq *models.OpenAIRequest) {
	openaiBody, err := cl.CreateChatCompletionStream(ctx, openaiReq)
	if err != nil {
		log.Printf("OpenAI API stream error: %v", err)
		http.Error(w, fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":"%s"}}`, err.Error()), http.StatusBadGateway)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"type":"error","error":{"type":"internal_error","message":"streaming not supported"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	flushFn := func(data []byte) error {
		_, err := w.Write(data)
		if err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := respConv.ConvertStreaming(ctx, openaiBody, flushFn); err != nil {
		log.Printf("stream conversion error: %v", err)
	}
}

func (h *Handler) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		System   string `json:"system,omitempty"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	totalChars := 0
	for _, msg := range req.Messages {
		totalChars += len(msg.Content)
	}
	totalChars += len(req.System)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"input_tokens": totalChars / 4,
	})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"proxy":   "claude-code-proxy",
		"version": "1.0.0",
	})
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfgMgr.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"proxy":       "claude-code-proxy",
		"version":     "1.0.0",
		"endpoints":   []string{"/v1/messages", "/v1/messages/count_tokens", "/health"},
		"model":       cfg.BigModel,
		"openai_base": cfg.OpenAIBaseURL,
	})
}

func (h *Handler) validateAPIKey(cfg *config.Config, r *http.Request) bool {
	if cfg.AnthropicAPIKey == "" {
		return true
	}

	key := r.Header.Get("x-api-key")
	if key == "" {
		if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			key = auth[7:]
		}
	}

	return key == cfg.AnthropicAPIKey
}
