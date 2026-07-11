package conversion

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/shukraditya/claude-code-proxy/models"
)

type ResponseConverter struct {
	originalModel string
}

func NewResponseConverter(originalModel string) *ResponseConverter {
	return &ResponseConverter{originalModel: originalModel}
}

func (c *ResponseConverter) ConvertNonStreaming(openaiResp *models.OpenAIResponse) *models.ClaudeResponse {
	claudeResp := &models.ClaudeResponse{
		ID:    fmt.Sprintf("msg_%s", openaiResp.ID),
		Type:  "message",
		Role:  "assistant",
		Model: c.originalModel,
	}

	for _, choice := range openaiResp.Choices {
		msg := choice.Message

		if msg.Content != "" {
			claudeResp.Content = append(claudeResp.Content, models.ClaudeContentBlock{
				Type: "text",
				Text: msg.Content,
			})
		}

		for _, tc := range msg.ToolCalls {
			claudeResp.Content = append(claudeResp.Content, models.ClaudeContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: parseJSONString(tc.Function.Arguments),
			})
		}

		if choice.FinishReason != nil {
			stopReason := mapFinishReason(*choice.FinishReason)
			claudeResp.StopReason = &stopReason
		}
	}

	if openaiResp.Usage != nil {
		claudeResp.Usage = models.ClaudeUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		}
		if openaiResp.Usage.PromptTokensDetails != nil && openaiResp.Usage.PromptTokensDetails.CachedTokens > 0 {
			ct := openaiResp.Usage.PromptTokensDetails.CachedTokens
			claudeResp.Usage.CacheReadInputTokens = &ct
		}
	}

	return claudeResp
}

func (c *ResponseConverter) ConvertStreaming(
	openaiBody io.ReadCloser,
	flushFn func([]byte) error,
) error {
	defer openaiBody.Close()

	messageID := fmt.Sprintf("msg_%08d", time.Now().UnixNano()%100000000)
	stopReason := ""
	toolAccumulators := map[int]*streamingToolAccum{}
	textBlockIndex := 0
	hasTextContent := false

	send := func(event string, data any) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		line := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(raw))
		return flushFn([]byte(line))
	}

	msg := map[string]any{
		"id":      messageID,
		"type":    "message",
		"role":    "assistant",
		"content": []any{},
		"model":   c.originalModel,
	}
	if err := send("message_start", map[string]any{"type": "message_start", "message": msg}); err != nil {
		return err
	}
	if err := send("ping", map[string]any{"type": "ping"}); err != nil {
		return err
	}

	reader := bufio.NewReader(openaiBody)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk models.OpenAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Printf("failed to unmarshal chunk: %v", err)
			continue
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			if delta.Content != "" {
				if !hasTextContent {
					hasTextContent = true
					block := map[string]any{"type": "text", "text": ""}
					if err := send("content_block_start", map[string]any{
						"type": "content_block_start", "index": textBlockIndex, "content_block": block,
					}); err != nil {
						return err
					}
				}
				if err := send("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": textBlockIndex,
					"delta": map[string]string{"type": "text_delta", "text": delta.Content},
				}); err != nil {
					return err
				}
			}

			for _, tc := range delta.ToolCalls {
				acc, ok := toolAccumulators[tc.Index]
				if !ok {
					acc = &streamingToolAccum{}
					toolAccumulators[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function != nil {
					if tc.Function.Name != "" {
						acc.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.arguments += tc.Function.Arguments
					}
				}
			}

			if choice.FinishReason != nil && *choice.FinishReason != "" {
				stopReason = *choice.FinishReason
			}
		}

		if chunk.Usage != nil {
			usage := map[string]any{
				"input_tokens":  chunk.Usage.PromptTokens,
				"output_tokens": chunk.Usage.CompletionTokens,
			}
			if chunk.Usage.PromptTokensDetails != nil && chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
				usage["cache_read_input_tokens"] = chunk.Usage.PromptTokensDetails.CachedTokens
			}
			stopReason = "end_turn"
			if err := send("message_delta", map[string]any{
				"type": "message_delta",
				"delta": map[string]string{
					"stop_reason": "end_turn", "stop_sequence": "",
				},
				"usage": usage,
			}); err != nil {
				return err
			}
		}
	}

	if hasTextContent {
		if err := send("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": textBlockIndex,
		}); err != nil {
			return err
		}
	}

	for idx, acc := range toolAccumulators {
		if acc.id == "" {
			continue
		}
		blockIndex := textBlockIndex + 1 + idx
		if err := send("content_block_start", map[string]any{
			"type": "content_block_start", "index": blockIndex,
			"content_block": map[string]any{"type": "tool_use", "id": acc.id, "name": acc.name},
		}); err != nil {
			return err
		}
		if acc.arguments != "" {
			if err := send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": acc.arguments},
			}); err != nil {
				return err
			}
		}
		if err := send("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": blockIndex,
		}); err != nil {
			return err
		}
	}

	if stopReason == "" {
		stopReason = "end_turn"
	}

	return send("message_stop", map[string]any{"type": "message_stop"})
}

type streamingToolAccum struct {
	id        string
	name      string
	arguments string
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return reason
	}
}

func parseJSONString(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}
