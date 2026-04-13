package aijudge

import (
	"testing"
)

func TestParseJudgeResponse_Clean(t *testing.T) {
	raw := `[{"text":"Submit","type":"button","rect":{"x":100,"y":200,"width":80,"height":30}},{"text":"Cancel","type":"button","rect":{"x":200,"y":200,"width":80,"height":30}}]`

	elems, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[0].Text != "Submit" {
		t.Errorf("first element text should be 'Submit', got %q", elems[0].Text)
	}
	if elems[0].Type != "button" {
		t.Errorf("first element type should be 'button', got %q", elems[0].Type)
	}
	if elems[0].Rect.X != 100 {
		t.Errorf("first element X should be 100, got %d", elems[0].Rect.X)
	}
}

func TestParseJudgeResponse_WithCodeFences(t *testing.T) {
	raw := "```json\n" +
		`[{"text":"OK","type":"button","rect":{"x":50,"y":50,"width":40,"height":20}}]` +
		"\n```"

	elems, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if elems[0].Text != "OK" {
		t.Errorf("element text should be 'OK', got %q", elems[0].Text)
	}
}

func TestParseJudgeResponse_WithPreamble(t *testing.T) {
	raw := `Here are the elements I found:

[{"text":"Hello","type":"text","rect":{"x":10,"y":10,"width":100,"height":20}}]

That's all the elements.`

	elems, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
}

func TestParseJudgeResponse_NoJSON(t *testing.T) {
	raw := "I can see a button that says Submit."

	_, err := parseJudgeResponse(raw)
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestParseJudgeResponse_EmptyArray(t *testing.T) {
	raw := "[]"

	elems, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(elems))
	}
}

func TestParseJudgeResponse_MalformedJSON(t *testing.T) {
	raw := `[{"text":"broken", "type"`

	_, err := parseJudgeResponse(raw)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseJudgeResponse_AllElementTypes(t *testing.T) {
	raw := `[
		{"text":"Page Title","type":"heading","rect":{"x":10,"y":10,"width":300,"height":40}},
		{"text":"Email:","type":"label","rect":{"x":10,"y":60,"width":50,"height":20}},
		{"text":"Enter email","type":"input","rect":{"x":70,"y":60,"width":200,"height":20}},
		{"text":"Submit","type":"button","rect":{"x":10,"y":100,"width":80,"height":30}},
		{"text":"Terms of Service","type":"link","rect":{"x":10,"y":150,"width":120,"height":15}},
		{"text":"Option A","type":"checkbox","rect":{"x":10,"y":180,"width":100,"height":20}},
		{"text":"Choice 1","type":"radio","rect":{"x":10,"y":210,"width":100,"height":20}},
		{"text":"Select...","type":"dropdown","rect":{"x":10,"y":240,"width":150,"height":30}},
		{"text":"ON","type":"toggle","rect":{"x":10,"y":280,"width":60,"height":24}},
		{"text":"50%","type":"slider","rect":{"x":10,"y":320,"width":200,"height":20}},
		{"text":"","type":"icon","rect":{"x":10,"y":360,"width":24,"height":24}},
		{"text":"Welcome paragraph","type":"text","rect":{"x":10,"y":400,"width":400,"height":60}},
		{"text":"42","type":"value","rect":{"x":10,"y":470,"width":30,"height":20}}
	]`

	elems, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 13 {
		t.Fatalf("expected 13 elements (all types), got %d", len(elems))
	}

	// Verify each type was parsed
	types := make(map[string]bool)
	for _, e := range elems {
		types[e.Type] = true
	}
	expectedTypes := []string{"heading", "label", "input", "button", "link", "checkbox", "radio", "dropdown", "toggle", "slider", "icon", "text", "value"}
	for _, et := range expectedTypes {
		if !types[et] {
			t.Errorf("missing type %q in parsed elements", et)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncate("hello world this is long", 10) != "hello worl..." {
		t.Errorf("long string should be truncated, got %q", truncate("hello world this is long", 10))
	}
}
