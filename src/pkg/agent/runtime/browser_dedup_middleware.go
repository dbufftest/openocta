package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stellarlinkco/agentsdk-go/pkg/middleware"
	"github.com/stellarlinkco/agentsdk-go/pkg/model"
)

// browserDedupRecord tracks the last browser navigation per session.
type browserDedupRecord struct {
	url       string
	timestamp time.Time
}

// browserDedupMiddleware prevents redundant browser navigate/open calls
// within a short time window for the same URL in the same session.
type browserDedupMiddleware struct {
	mu      sync.RWMutex
	history map[string]browserDedupRecord // key: sessionID
	window  time.Duration
}

// newBrowserDedupMiddleware creates a middleware that deduplicates browser
// navigation tool calls. The window specifies how long a previous navigation
// to the same URL is considered "recent" and will block duplicates.
func newBrowserDedupMiddleware(window time.Duration) middleware.Middleware {
	if window <= 0 {
		window = 30 * time.Second
	}
	return &browserDedupMiddleware{
		history: make(map[string]browserDedupRecord),
		window:  window,
	}
}

func (m *browserDedupMiddleware) Name() string {
	return "openocta-browser-dedup"
}

func (m *browserDedupMiddleware) BeforeAgent(ctx context.Context, st *middleware.State) error {
	return nil
}

func (m *browserDedupMiddleware) BeforeTool(ctx context.Context, st *middleware.State) error {
	if st == nil {
		return nil
	}
	call, ok := st.ToolCall.(model.ToolCall)
	if !ok {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(call.Name))
	if !isBrowserNavigationTool(name) {
		return nil
	}
	url := extractURLFromToolCall(call)
	if url == "" {
		return nil
	}

	sessionID, _ := st.Values["session_id"].(string)
	if sessionID == "" {
		return nil
	}

	m.mu.RLock()
	last, exists := m.history[sessionID]
	m.mu.RUnlock()

	if exists && strings.EqualFold(last.url, url) && time.Since(last.timestamp) < m.window {
		return fmt.Errorf("browser dedup: navigation to %s was already performed %v ago; reuse the existing page instead of re-navigating", url, time.Since(last.timestamp).Round(time.Second))
	}

	return nil
}

func (m *browserDedupMiddleware) AfterTool(ctx context.Context, st *middleware.State) error {
	if st == nil {
		return nil
	}
	call, ok := st.ToolCall.(model.ToolCall)
	if !ok {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(call.Name))
	if !isBrowserNavigationTool(name) {
		return nil
	}
	url := extractURLFromToolCall(call)
	if url == "" {
		return nil
	}

	sessionID, _ := st.Values["session_id"].(string)
	if sessionID == "" {
		return nil
	}

	m.mu.Lock()
	m.history[sessionID] = browserDedupRecord{url: url, timestamp: time.Now()}
	m.mu.Unlock()

	return nil
}

func (m *browserDedupMiddleware) AfterAgent(ctx context.Context, st *middleware.State) error {
	return nil
}

// isBrowserNavigationTool returns true if the tool name indicates a page
// navigation/open operation.
func isBrowserNavigationTool(name string) bool {
	switch name {
	case "browser_navigate", "browser_open", "browser_goto",
		"playwright_navigate", "playwright_open", "playwright_goto",
		"navigate", "open", "goto",
		"browser", "playwright":
		return true
	}
	// Also match prefixed variants like "mcp-playwright-navigate" etc.
	if strings.Contains(name, "navigate") || strings.Contains(name, "_goto") {
		return true
	}
	return false
}

// extractURLFromToolCall tries to extract a target URL from common browser
// navigation tool argument schemas.
func extractURLFromToolCall(call model.ToolCall) string {
	for _, key := range []string{"url", "targetUrl", "target_url", "href", "link", "page", "address"} {
		if v, ok := call.Arguments[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	// Some tools nest URL under a "request" object.
	if req, ok := call.Arguments["request"].(map[string]interface{}); ok {
		for _, key := range []string{"url", "targetUrl", "target_url"} {
			if v, ok := req[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}
