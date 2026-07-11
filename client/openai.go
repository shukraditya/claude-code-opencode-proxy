package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shukraditya/claude-code-proxy/config"
	"github.com/shukraditya/claude-code-proxy/models"
)

type OpenAIClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewOpenAIClient(cfg *config.Config) *OpenAIClient {
	return &OpenAIClient{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
}

func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req *models.OpenAIRequest) (*models.OpenAIResponse, error) {
	body, err := c.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.OpenAIBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.classifyError(resp.StatusCode, respBody)
	}

	var openaiResp models.OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &openaiResp, nil
}

func (c *OpenAIClient) CreateChatCompletionStream(ctx context.Context, req *models.OpenAIRequest) (io.ReadCloser, error) {
	req.Stream = true
	body, err := c.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.OpenAIBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, c.classifyError(resp.StatusCode, respBody)
	}

	return resp.Body, nil
}

func (c *OpenAIClient) buildRequestBody(req *models.OpenAIRequest) ([]byte, error) {
	if req.Stream {
		req.StreamOptions = &models.OpenAIStreamOptions{IncludeUsage: true}
	}

	hasMultiContent := false
	for _, msg := range req.Messages {
		if msg.MultiContent != nil {
			hasMultiContent = true
			break
		}
	}

	if !hasMultiContent {
		return json.Marshal(req)
	}

	multiReq := models.OpenAIMultiContentRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	multiReq.Messages = make([]models.OpenAIMultiMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		mm := models.OpenAIMultiMessage{
			Role:       msg.Role,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		if msg.MultiContent != nil {
			mm.Content = msg.MultiContent
		} else if msg.Content != "" {
			mm.Content = []models.OpenAIContentPart{
				{Type: "text", Text: msg.Content},
			}
		}
		multiReq.Messages = append(multiReq.Messages, mm)
	}

	return json.Marshal(multiReq)
}

func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	c.cfg.AddCustomHeaders(req)
}

func (c *OpenAIClient) classifyError(statusCode int, body []byte) error {
	var errBody struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errBody); err == nil && errBody.Error.Message != "" {
		return fmt.Errorf("OpenAI API error (HTTP %d): %s", statusCode, errBody.Error.Message)
	}
	return fmt.Errorf("OpenAI API error (HTTP %d): %s", statusCode, string(body))
}
