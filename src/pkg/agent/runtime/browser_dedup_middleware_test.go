package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stellarlinkco/agentsdk-go/pkg/middleware"
	"github.com/stellarlinkco/agentsdk-go/pkg/model"
)

func TestBrowserDedupMiddlewareBlocksDuplicate(t *testing.T) {
	m := newBrowserDedupMiddleware(5 * time.Second)
	ctx := context.Background()

	st1 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://example.com/login"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}
	st2 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://example.com/login"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}

	// First call should be allowed.
	if err := m.BeforeTool(ctx, st1); err != nil {
		t.Fatalf("first call should be allowed: %v", err)
	}
	// Record the navigation.
	_ = m.AfterTool(ctx, st1)

	// Immediate duplicate should be blocked.
	if err := m.BeforeTool(ctx, st2); err == nil {
		t.Fatal("duplicate call should be blocked")
	} else if !strings.Contains(err.Error(), "browser dedup") {
		t.Fatalf("expected dedup error, got: %v", err)
	}
}

func TestBrowserDedupMiddlewareAllowsDifferentURL(t *testing.T) {
	m := newBrowserDedupMiddleware(5 * time.Second)
	ctx := context.Background()

	st1 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://a.com"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}
	st2 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://b.com"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}

	_ = m.BeforeTool(ctx, st1)
	_ = m.AfterTool(ctx, st1)

	if err := m.BeforeTool(ctx, st2); err != nil {
		t.Fatalf("different URL should be allowed: %v", err)
	}
}

func TestBrowserDedupMiddlewareAllowsAfterWindow(t *testing.T) {
	m := newBrowserDedupMiddleware(50 * time.Millisecond)
	ctx := context.Background()

	st1 := &middleware.State{
		ToolCall: model.ToolCall{Name: "browser_navigate", Arguments: map[string]any{"url": "https://example.com"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}
	st2 := &middleware.State{
		ToolCall: model.ToolCall{Name: "browser_navigate", Arguments: map[string]any{"url": "https://example.com"}},
		Values:   map[string]any{"session_id": "sess-1"},
	}

	_ = m.BeforeTool(ctx, st1)
	_ = m.AfterTool(ctx, st1)

	time.Sleep(100 * time.Millisecond)

	if err := m.BeforeTool(ctx, st2); err != nil {
		t.Fatalf("call after window should be allowed: %v", err)
	}
}

func TestBrowserDedupMiddlewareDifferentSessions(t *testing.T) {
	m := newBrowserDedupMiddleware(5 * time.Second)
	ctx := context.Background()

	st1 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://example.com"}},
		Values:   map[string]any{"session_id": "sess-a"},
	}
	st2 := &middleware.State{
		ToolCall: model.ToolCall{Name: "playwright_navigate", Arguments: map[string]any{"url": "https://example.com"}},
		Values:   map[string]any{"session_id": "sess-b"},
	}

	_ = m.BeforeTool(ctx, st1)
	_ = m.AfterTool(ctx, st1)

	if err := m.BeforeTool(ctx, st2); err != nil {
		t.Fatalf("different session should be allowed: %v", err)
	}
}

func TestExtractURLFromToolCall(t *testing.T) {
	tests := []struct {
		name string
		call model.ToolCall
		want string
	}{
		{
			name: "url key",
			call: model.ToolCall{Arguments: map[string]any{"url": "https://a.com"}},
			want: "https://a.com",
		},
		{
			name: "targetUrl key",
			call: model.ToolCall{Arguments: map[string]any{"targetUrl": "https://b.com"}},
			want: "https://b.com",
		},
		{
			name: "nested request",
			call: model.ToolCall{Arguments: map[string]any{"request": map[string]interface{}{"url": "https://c.com"}}},
			want: "https://c.com",
		},
		{
			name: "no url",
			call: model.ToolCall{Arguments: map[string]any{"foo": "bar"}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLFromToolCall(tt.call)
			if got != tt.want {
				t.Fatalf("extractURLFromToolCall() = %q, want %q", got, tt.want)
			}
		})
	}
}
