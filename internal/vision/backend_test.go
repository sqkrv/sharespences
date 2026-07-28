package vision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaComplete(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"message":{"content":"{\"ok\":true}","thinking":"hm"},"done_reason":"stop"}`))
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "qwen3-vl:4b")
	resp, err := o.Complete(context.Background(), Request{
		Prompt: "read this", ImageJPEG: []byte{0xff, 0xd8}, Schema: SlotSchema, NumPredict: 2048, NumCtx: 16384, Think: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"ok":true}` || resp.Thinking != "hm" || resp.DoneReason != "stop" {
		t.Fatalf("resp = %+v", resp)
	}
	if got["model"] != "qwen3-vl:4b" || got["stream"] != false || got["think"] != true {
		t.Fatalf("request body = %v", got)
	}
	opts := got["options"].(map[string]any)
	if opts["temperature"] != 0.0 || opts["num_ctx"] != 16384.0 || opts["num_predict"] != 2048.0 {
		t.Fatalf("options = %v", opts)
	}
	if got["format"] == nil {
		t.Fatal("schema not passed as format")
	}
	msg := got["messages"].([]any)[0].(map[string]any)
	if imgs := msg["images"].([]any); len(imgs) != 1 || imgs[0] != "/9g=" {
		t.Fatalf("images = %v", msg["images"])
	}
}

func TestOllamaDefaultsNumCtx(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"message":{"content":"x"}}`))
	}))
	defer srv.Close()
	if _, err := NewOllama(srv.URL, "m").Complete(context.Background(), Request{Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	opts := got["options"].(map[string]any)
	if opts["num_ctx"] != float64(defaultNumCtx) {
		t.Fatalf("num_ctx = %v, want %d", opts["num_ctx"], defaultNumCtx)
	}
	if _, has := opts["num_predict"]; has {
		t.Fatal("num_predict must be omitted when zero")
	}
}

func TestOllamaOOMBecomesBackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"cudaMalloc failed: out of memory"}`))
	}))
	defer srv.Close()
	_, err := NewOllama(srv.URL, "m").Complete(context.Background(), Request{Prompt: "p"})
	var be *BackendError
	if !errors.As(err, &be) || be.Status != 500 {
		t.Fatalf("want *BackendError 500, got %v", err)
	}
	if !oomSignature(err) {
		t.Fatalf("OOM signature not recognized in %v", err)
	}
}

func TestOllamaModelMissingIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model \"qwen3-vl:4b\" not found, try pulling it first"}`))
	}))
	defer srv.Close()
	_, err := NewOllama(srv.URL, "qwen3-vl:4b").Complete(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestOllamaDownIsUnavailable(t *testing.T) {
	// Constructing must not dial (the openapi CI gate depends on that);
	// only Complete may fail.
	o := NewOllama("http://127.0.0.1:1", "m")
	_, err := o.Complete(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestAnthropicComplete(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		hdr = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"{\"slot_count\":4"},{"type":"text","text":",\"source_text\":\"x\"}"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	a := NewAnthropic("test-key", "")
	a.BaseURL = srv.URL
	resp, err := a.Complete(context.Background(), Request{Prompt: "read", ImageJPEG: []byte{1}, Schema: SlotSchema})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"slot_count":4,"source_text":"x"}` || resp.DoneReason != "end_turn" {
		t.Fatalf("resp = %+v", resp)
	}
	if hdr.Get("x-api-key") != "test-key" || hdr.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("headers = %v", hdr)
	}
	if got["model"] != "claude-opus-5" {
		t.Fatalf("model = %v, want the claude-opus-5 default", got["model"])
	}
	if got["output_config"] == nil {
		t.Fatal("schema must map to output_config.format")
	}
	blocks := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if blocks[0].(map[string]any)["type"] != "image" || blocks[1].(map[string]any)["type"] != "text" {
		t.Fatalf("content order = %v, want image before text", blocks)
	}
}

func TestAnthropicAuthErrorIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()
	a := NewAnthropic("bad", "claude-opus-5")
	a.BaseURL = srv.URL
	_, err := a.Complete(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestAnthropicRateLimitIsBackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer srv.Close()
	a := NewAnthropic("k", "claude-opus-5")
	a.BaseURL = srv.URL
	_, err := a.Complete(context.Background(), Request{Prompt: "p"})
	var be *BackendError
	if !errors.As(err, &be) || be.Status != 429 || be.Detail != "slow down" {
		t.Fatalf("want *BackendError 429 «slow down», got %v", err)
	}
}
