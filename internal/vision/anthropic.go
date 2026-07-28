package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Anthropic is the hosted fallback backend and the accuracy ceiling the
// local models are measured against. Deliberately stdlib-only and
// symmetric with the Ollama adapter — no SDK dependency for one endpoint.
// Never dials at construction.
type Anthropic struct {
	BaseURL string // default https://api.anthropic.com
	APIKey  string
	Model   string // default claude-opus-5
	client  *http.Client
}

func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = "claude-opus-5"
	}
	return &Anthropic{BaseURL: "https://api.anthropic.com", APIKey: apiKey, Model: model, client: &http.Client{}}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.Model }

type anthropicContent struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *anthropicSource `json:"source,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string             `json:"role"`
		Content []anthropicContent `json:"content"`
	} `json:"messages"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
	Fallbacks    string                 `json:"fallbacks,omitempty"`
}

type anthropicOutputConfig struct {
	Format struct {
		Type   string          `json:"type"`
		Schema json.RawMessage `json:"schema"`
	} `json:"format"`
}

type anthropicResponse struct {
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete maps Request onto POST /v1/messages. Request.Think is ignored:
// the model thinks adaptively by default and the ladder's think toggle is
// an Ollama-ism. Request.Schema becomes a structured-output constraint.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	maxTokens := req.NumPredict
	if maxTokens == 0 {
		maxTokens = 8192
	}
	body := anthropicRequest{Model: a.Model, MaxTokens: maxTokens, Fallbacks: "default"}
	var content []anthropicContent
	if req.ImageJPEG != nil {
		content = append(content, anthropicContent{Type: "image", Source: &anthropicSource{
			Type: "base64", MediaType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(req.ImageJPEG),
		}})
	}
	content = append(content, anthropicContent{Type: "text", Text: req.Prompt})
	body.Messages = append(body.Messages, struct {
		Role    string             `json:"role"`
		Content []anthropicContent `json:"content"`
	}{Role: "user", Content: content})
	if req.Schema != nil {
		oc := &anthropicOutputConfig{}
		oc.Format.Type = "json_schema"
		oc.Format.Schema = req.Schema
		body.OutputConfig = oc
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	// Server-side refusal fallback: a safety-classifier decline is
	// re-served by the recommended fallback model inside the same call.
	httpReq.Header.Set("anthropic-beta", "server-side-fallback-2026-07-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		return Response{}, fmt.Errorf("%w: anthropic: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrFailed, err)
	}

	var parsed anthropicResponse
	_ = json.Unmarshal(data, &parsed)
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		// Bad/missing key or unknown model — deployment problems.
		return Response{}, fmt.Errorf("%w: anthropic: %s", ErrUnavailable, errDetail(parsed, data))
	default:
		return Response{}, &BackendError{Backend: a.Name(), Status: resp.StatusCode, Detail: errDetail(parsed, data)}
	}
	if parsed.StopReason == "refusal" {
		return Response{}, &BackendError{Backend: a.Name(), Status: resp.StatusCode, Detail: "model refused the request"}
	}

	var out Response
	out.DoneReason = parsed.StopReason
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			out.Content += block.Text
		case "thinking":
			out.Thinking += block.Thinking
		}
	}
	return out, nil
}

func errDetail(parsed anthropicResponse, raw []byte) string {
	if parsed.Error != nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	return string(raw)
}
