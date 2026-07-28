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

// Ollama is the default, local-first backend (bench.py chat() port —
// /api/chat, non-streaming, temperature 0). It never dials at
// construction; the first Complete call is the first network touch.
type Ollama struct {
	BaseURL string // e.g. http://localhost:11434
	Model   string // e.g. qwen3-vl:4b
	client  *http.Client
}

func NewOllama(baseURL, model string) *Ollama {
	return &Ollama{BaseURL: baseURL, Model: model, client: &http.Client{}}
}

func (o *Ollama) Name() string { return "ollama:" + o.Model }

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Think    bool            `json:"think"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	DoneReason string `json:"done_reason"`
	Error      string `json:"error"`
}

func (o *Ollama) Complete(ctx context.Context, req Request) (Response, error) {
	msg := ollamaMessage{Role: "user", Content: req.Prompt}
	if req.ImageJPEG != nil {
		msg.Images = []string{base64.StdEncoding.EncodeToString(req.ImageJPEG)}
	}
	numCtx := req.NumCtx
	if numCtx == 0 {
		numCtx = defaultNumCtx
	}
	body, err := json.Marshal(ollamaRequest{
		Model:    o.Model,
		Messages: []ollamaMessage{msg},
		Stream:   false,
		Think:    req.Think,
		Format:   req.Schema,
		Options:  ollamaOptions{Temperature: 0, NumCtx: numCtx, NumPredict: req.NumPredict},
	})
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		return Response{}, fmt.Errorf("%w: ollama at %s: %v", ErrUnavailable, o.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrFailed, err)
	}

	var parsed ollamaResponse
	// Ollama puts the real reason in the body's error field, on error
	// statuses and occasionally with a 200.
	_ = json.Unmarshal(data, &parsed)
	if resp.StatusCode == http.StatusNotFound {
		// Model not pulled — a deployment problem, not a per-image one.
		return Response{}, fmt.Errorf("%w: ollama: %s", ErrUnavailable, firstNonEmpty(parsed.Error, "model not found"))
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, &BackendError{Backend: o.Name(), Status: resp.StatusCode, Detail: firstNonEmpty(parsed.Error, string(data))}
	}
	if parsed.Error != "" {
		return Response{}, &BackendError{Backend: o.Name(), Status: resp.StatusCode, Detail: parsed.Error}
	}
	return Response{Content: parsed.Message.Content, Thinking: parsed.Message.Thinking, DoneReason: parsed.DoneReason}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
