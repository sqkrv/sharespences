package vision

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Port of bench.py:_extract_json. Thinking models fail three ways a bare
// json.Unmarshal cannot survive: empty content (the whole budget went to
// reasoning), a leaked <think> block inside content, and markdown-fenced
// JSON. Each measured on the corpus — see the harness README.
var (
	thinkClosed = regexp.MustCompile(`(?s)<think>.*?</think>`)
	thinkOpen   = regexp.MustCompile(`(?s)^<think>.*`) // unterminated block
	fence       = regexp.MustCompile("(?s)```(?:json)?\\s*(.+?)```")
)

// ExtractJSON pulls the first JSON object out of the given texts (content
// first, then thinking). Returns nil when no text yields one.
func ExtractJSON(texts ...string) json.RawMessage {
	for _, t := range texts {
		if strings.TrimSpace(t) == "" {
			continue
		}
		t = strings.TrimSpace(thinkClosed.ReplaceAllString(t, ""))
		t = strings.TrimSpace(thinkOpen.ReplaceAllString(t, ""))
		if m := fence.FindStringSubmatch(t); m != nil {
			t = strings.TrimSpace(m[1])
		}
		if obj := firstObject(t); obj != nil {
			return obj
		}
	}
	return nil
}

// firstObject scans for the first balanced {...}. Unlike the harness, the
// scan is string-aware — a brace inside a title («Кафе {и} бары») must not
// break the balance count.
func firstObject(t string) json.RawMessage {
	start := strings.IndexByte(t, '{')
	if start < 0 {
		return nil
	}
	depth, inString, escaped := 0, false, false
	for j := start; j < len(t); j++ {
		c := t[j]
		switch {
		case escaped:
			escaped = false
		case inString:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				if cand := json.RawMessage(t[start : j+1]); json.Valid(cand) {
					return cand
				}
				return nil // mirror the harness: one failed parse → next text
			}
		}
	}
	return nil
}
