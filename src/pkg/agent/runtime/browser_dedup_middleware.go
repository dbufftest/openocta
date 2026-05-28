package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/stellarlinkco/agentsdk-go/pkg/middleware"
	"github.com/stellarlinkco/agentsdk-go/pkg/model"
)

// browserDedupState holds the deduplication window and seen URLs.
type browserDedupState struct {
	window time.Duration
	mu     sync.Mutex
	seen   map[string]time.Time
}

// newBrowserDedupMiddleware prevents the LLM from repeatedly opening the same URL
// within a short window during multi-step UI automation.
func newBrowserDedupMiddleware(windowSecs int) middleware.Middleware {
	window := time.Duration(windowSecs) * time.Second
	if window <= 0 {
		window = 30 * time.Second
	}
	state := &browserDedupState{
		window: window,
		seen:   make(map[string]time.Time),
	}
	return middleware.Funcs{
		Identifier: "openocta-browser-dedup",
		OnBeforeTool: func(_ context.Context, st *middleware.State) error {
			if st == nil {
				return nil
			}
			call, ok := st.ToolCall.(model.ToolCall)
			if !ok {
				return nil
			}
			// Only deduplicate browser/navigation tools.
			name := strings.ToLower(strings.TrimSpace(call.Name))
			if name != "browser" && name != "navigate" && name != "open_url" {
				return nil
			}
			url, _ := call.Arguments["url"].(string)
			if strings.TrimSpace(url) == "" {
				url, _ = call.Arguments["command"].(string)
			}
			url = strings.TrimSpace(url)
			if url == "" {
				return nil
			}

			state.mu.Lock()
			defer state.mu.Unlock()
			if last, ok := state.seen[url]; ok && time.Since(last) < state.window {
				return nil // silently skip duplicate within window
			}
			state.seen[url] = time.Now()
			return nil
		},
	}
}
