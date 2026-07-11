package conversion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shukraditya/claude-code-proxy/config"
	"github.com/shukraditya/claude-code-proxy/models"
)

type RequestConverter struct {
	cfg *config.Config
}

func NewRequestConverter(cfg *config.Config) *RequestConverter {
	return &RequestConverter{cfg: cfg}
}

func (c *RequestConverter) Convert(claudeReq *models.ClaudeMessagesRequest) (*models.OpenAIRequest, string, error) {
	mappedModel := c.mapModel(claudeReq.Model)
	openaiReq := &models.OpenAIRequest{
		Model:       mappedModel,
		Stream:      claudeReq.Stream,
		Temperature: claudeReq.Temperature,
		TopP:        claudeReq.TopP,
		MaxTokens:   claudeReq.MaxTokens,
	}

	if len(claudeReq.StopSequences) > 0 {
		openaiReq.Stop = claudeReq.StopSequences
	}

	if err := c.convertSystem(claudeReq.System, openaiReq); err != nil {
		return nil, "", fmt.Errorf("convert system: %w", err)
	}

	if err := c.convertMessages(claudeReq.Messages, openaiReq); err != nil {
		return nil, "", fmt.Errorf("convert messages: %w", err)
	}

	if len(claudeReq.Tools) > 0 {
		openaiReq.Tools = c.convertTools(claudeReq.Tools)
	}

	if claudeReq.ToolChoice != nil {
		openaiReq.ToolChoice = c.convertToolChoice(claudeReq.ToolChoice)
	}

	if claudeReq.Stream {
		openaiReq.StreamOptions = &models.OpenAIStreamOptions{IncludeUsage: true}
	}

	return openaiReq, mappedModel, nil
}

func (c *RequestConverter) mapModel(claudeModel string) string {
	return c.cfg.BigModel
}

func (c *RequestConverter) convertSystem(system json.RawMessage, req *models.OpenAIRequest) error {
	if len(system) == 0 {
		return nil
	}

	var textParts []string

	var systemStr string
	if err := json.Unmarshal(system, &systemStr); err == nil {
		if systemStr != "" {
			req.Messages = append(req.Messages, models.OpenAIMessage{
				Role:    "system",
				Content: systemStr,
			})
		}
		return nil
	}

	var blocks []models.ClaudeSystemContent
	if err := json.Unmarshal(system, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" {
				textParts = append(textParts, b.Text)
			}
		}
	}

	if len(textParts) > 0 {
		req.Messages = append(req.Messages, models.OpenAIMessage{
			Role:    "system",
			Content: strings.Join(textParts, "\n"),
		})
	}

	return nil
}

func (c *RequestConverter) convertMessages(claudeMessages []models.ClaudeMessage, req *models.OpenAIRequest) error {
	var pendingToolResults []models.OpenAIMessage

	for i, msg := range claudeMessages {
		role := msg.Role

		content := strings.TrimSpace(string(msg.Content))
		isToolResult := false
		if content != "" && content[0] == '[' {
			var blocks []models.ClaudeContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "tool_result" {
						isToolResult = true
						break
					}
				}
			}
		}

		if isToolResult {
			var blocks []models.ClaudeContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "tool_result" {
						toolMsg := c.convertToolResult(b)
						if toolMsg != nil {
							pendingToolResults = append(pendingToolResults, *toolMsg)
						}
					}
				}
			}
			continue
		}

		if len(pendingToolResults) > 0 {
			req.Messages = append(req.Messages, pendingToolResults...)
			pendingToolResults = nil
		}

		switch role {
		case "user":
			converted, err := c.convertUserMessage(msg)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			req.Messages = append(req.Messages, *converted)

		case "assistant":
			converted := c.convertAssistantMessage(msg)
			req.Messages = append(req.Messages, *converted)

		default:
			req.Messages = append(req.Messages, models.OpenAIMessage{
				Role:    role,
				Content: strings.Trim(string(msg.Content), "\""),
			})
		}
	}

	if len(pendingToolResults) > 0 {
		req.Messages = append(req.Messages, pendingToolResults...)
	}

	return nil
}

func (c *RequestConverter) convertUserMessage(msg models.ClaudeMessage) (*models.OpenAIMessage, error) {
	content := strings.TrimSpace(string(msg.Content))

	if content == "" {
		return &models.OpenAIMessage{Role: "user", Content: ""}, nil
	}

	if content[0] != '[' {
		return &models.OpenAIMessage{
			Role:    "user",
			Content: strings.Trim(content, "\""),
		}, nil
	}

	var blocks []models.ClaudeContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return &models.OpenAIMessage{
			Role:    "user",
			Content: strings.Trim(content, "\""),
		}, nil
	}

	if len(blocks) == 1 && blocks[0].Type == "text" {
		return &models.OpenAIMessage{
			Role:    "user",
			Content: blocks[0].Text,
		}, nil
	}

	var parts []models.OpenAIContentPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, models.OpenAIContentPart{
				Type: "text",
				Text: b.Text,
			})
		case "image":
			if b.Source != nil && b.Source.Type == "base64" {
				parts = append(parts, models.OpenAIContentPart{
					Type: "image_url",
					ImageURL: &models.OpenAIImageURL{
						URL: fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data),
					},
				})
			}
		}
	}

	if len(parts) == 0 {
		return &models.OpenAIMessage{Role: "user", Content: ""}, nil
	}

	if len(parts) == 1 && parts[0].Type == "text" {
		return &models.OpenAIMessage{
			Role:    "user",
			Content: parts[0].Text,
		}, nil
	}

	return &models.OpenAIMessage{
		Role:    "user",
		Content: "",
		MultiContent: parts,
	}, nil
}

func (c *RequestConverter) convertAssistantMessage(msg models.ClaudeMessage) *models.OpenAIMessage {
	content := strings.TrimSpace(string(msg.Content))

	if content == "" {
		return &models.OpenAIMessage{Role: "assistant"}
	}

	if content[0] != '[' {
		return &models.OpenAIMessage{
			Role:    "assistant",
			Content: strings.Trim(content, "\""),
		}
	}

	var blocks []models.ClaudeContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return &models.OpenAIMessage{
			Role:    "assistant",
			Content: strings.Trim(content, "\""),
		}
	}

	var textParts []string
	var toolCalls []models.OpenAIToolCall

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "tool_use":
			inputBytes, err := json.Marshal(b.Input)
			if err != nil {
				continue
			}
			toolCalls = append(toolCalls, models.OpenAIToolCall{
				ID:   b.ID,
				Type: "function",
				Function: models.OpenAIFunction{
					Name:      b.Name,
					Arguments: string(inputBytes),
				},
			})
		}
	}

	msg2 := &models.OpenAIMessage{Role: "assistant"}
	if len(textParts) > 0 {
		msg2.Content = strings.Join(textParts, "\n")
	}
	if len(toolCalls) > 0 {
		msg2.ToolCalls = toolCalls
	}

	return msg2
}

func (c *RequestConverter) convertToolResult(block models.ClaudeContentBlock) *models.OpenAIMessage {
	content := parseToolResultContent(block.Content)
	return &models.OpenAIMessage{
		Role:       "tool",
		ToolCallID: block.ToolUseID,
		Content:    content,
	}
}

func parseToolResultContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if t, ok := v["text"].(string); ok {
			return t
		}
		b, _ := json.Marshal(v)
		return string(b)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func (c *RequestConverter) convertTools(tools []models.ClaudeTool) []models.OpenAITool {
	result := make([]models.OpenAITool, 0, len(tools))
	for _, t := range tools {
		result = append(result, models.OpenAITool{
			Type: "function",
			Function: models.OpenAIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return result
}

func (c *RequestConverter) convertToolChoice(choice *models.ClaudeToolChoice) any {
	switch choice.Type {
	case "any":
		return "auto"
	case "auto":
		return "auto"
	case "tool":
		if choice.Name != "" {
			return models.OpenAIToolChoice{
				Type: "function",
				Function: &models.OpenAIToolChoiceFunction{
					Name: choice.Name,
				},
			}
		}
		return "auto"
	case "none":
		return "none"
	default:
		return "auto"
	}
}


