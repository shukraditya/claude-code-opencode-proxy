# claude-code-opencode-proxy

A Go proxy that translates Anthropic Messages API (used by Claude Code) to OpenAI-compatible API calls. Lets you use Claude Code with any OpenAI-compatible backend — including models available through your opencode subscription.

## How it works

```
Claude Code  ──►  Proxy (:8082)  ──►  OpenAI-compatible API
(Anthropic         translates            (opencode.ai/zen/go/v1
 Messages API)     Anthropic→OpenAI       deepseek-v4-flash, etc.)
```

Claude Code sends requests in Anthropic's format. The proxy converts them to OpenAI format, forwards them to your configured backend, then translates the streaming response back to Anthropic SSE events.

## Quick start

```bash
# 1. Copy config
cp .env.example .env

# 2. Edit .env — fill in your API key
OPENAI_API_KEY="your-key-here"

# 3. Run
go run .

# 4. Use with Claude Code
ANTHROPIC_BASE_URL=http://localhost:8082 ANTHROPIC_API_KEY=any-value claude
```

## Configuration

Set these in `.env` (auto-reloaded on change, no restart needed):

| Var | Default | Description |
|-----|---------|-------------|
| `OPENAI_API_KEY` | — | API key for the OpenAI-compatible backend |
| `BIG_MODEL` | `deepseek-v4-flash` | Model to use (see registry below) |
| `HOST` | `0.0.0.0` | Listen address |
| `PORT` | `8082` | Listen port |

## Model registry

The proxy has a built-in registry mapping model IDs to their correct endpoints. Just set `BIG_MODEL` to any of these and the endpoint is handled automatically:

### OpenAI-compatible (translated from Anthropic format)

| Model ID | Endpoint |
|----------|----------|
| `deepseek-v4-flash` | `https://opencode.ai/zen/go/v1` |
| `deepseek-v4-pro` | `https://opencode.ai/zen/go/v1` |
| `glm-5.2` | `https://opencode.ai/zen/go/v1` |
| `glm-5.1` | `https://opencode.ai/zen/go/v1` |
| `kimi-k2.7-code` | `https://opencode.ai/zen/go/v1` |
| `kimi-k2.6` | `https://opencode.ai/zen/go/v1` |
| `mimo-v2.5` | `https://opencode.ai/zen/go/v1` |
| `mimo-v2.5-pro` | `https://opencode.ai/zen/go/v1` |

### Anthropic-native (passthrough, no translation)

| Model ID | Endpoint |
|----------|----------|
| `minimax-m3` | `https://opencode.ai/zen/go/v1` |
| `minimax-m2.7` | `https://opencode.ai/zen/go/v1` |
| `minimax-m2.5` | `https://opencode.ai/zen/go/v1` |
| `qwen3.7-max` | `https://opencode.ai/zen/go/v1` |
| `qwen3.7-plus` | `https://opencode.ai/zen/go/v1` |
| `qwen3.6-plus` | `https://opencode.ai/zen/go/v1` |

If your model isn't listed, set `OPENAI_BASE_URL` explicitly in `.env` to override the registry.

## Features

- **SSE streaming** — real-time token-by-token response, just like native Claude Code
- **Tool use** — function calling converted between Anthropic tool_use/tool_result and OpenAI tool_calls
- **Image support** — base64 images in Anthropic format converted to OpenAI image_url
- **Config auto-reload** — edit `.env` and changes are picked up within ~3 seconds; no restart needed
- **No token limits** — `max_tokens` from Claude Code is passed through as-is
- **Context window** — response returns the actual model name so Claude Code knows the real context window

## Project structure

```
├── main.go              # Entry point
├── config/config.go     # Configuration, env loading, auto-reload, model registry
├── models/
│   ├── claude.go        # Anthropic Messages API types
│   └── openai.go        # OpenAI Chat Completions types
├── conversion/
│   ├── request.go       # Claude → OpenAI request conversion
│   └── response.go      # OpenAI → Claude response conversion (streaming + non-streaming)
├── client/openai.go     # OpenAI HTTP client
└── proxy/handler.go     # HTTP handler
```
