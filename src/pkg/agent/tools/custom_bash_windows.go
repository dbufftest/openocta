//go:build windows

package tools

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"

	"github.com/stellarlinkco/agentsdk-go/pkg/tool"
)

// CustomBashTool executes bash commands on Windows via cmd.exe to avoid window flashing.
// This wraps the command with cmd.exe to ensure proper window hiding.
type CustomBashTool struct {
	Timeout time.Duration
}

func (CustomBashTool) Name() string {
	return "bash"
}

func (CustomBashTool) Description() string {
	return "Execute a bash command on Windows. Uses cmd.exe wrapper to ensure silent execution without window flashing."
}

func (CustomBashTool) Schema() *tool.JSONSchema {
	return &tool.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Optional timeout in seconds (default 600)",
			},
		},
		Required: []string{"command"},
	}
}

func (t CustomBashTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	cmdStr, _ := params["command"].(string)
	if cmdStr == "" {
		return &tool.ToolResult{Success: false, Output: "command is required"}, nil
	}

	// Detect interactive commands that will hang waiting for stdin
	if isInteractiveCommand(cmdStr) {
		return &tool.ToolResult{
			Success: false,
			Output:  "Interactive command detected. Commands like 'ssh', 'mysql', 'redis-cli' require interactive input and will hang. Use non-interactive alternatives: ssh with sshpass or SSH keys, mysql with -e flag, etc.",
		}, nil
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if timeoutSec, ok := params["timeout"].(float64); ok && timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	// Use cmd.exe to wrap bash command, ensuring proper window hiding
	cmd := exec.CommandContext(timeoutCtx, "cmd.exe", "/d", "/s", "/c", "bash", "-c", cmdStr)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x00040000 | 0x00000008,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil

	err := cmd.Run()

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return &tool.ToolResult{Success: false, Output: "command timeout after " + timeout.String()}, nil
	}

	outStr := stdout.String()
	errStr := stderr.String()

	if err != nil {
		combined := outStr
		if errStr != "" {
			if combined != "" {
				combined += "\n"
			}
			combined += errStr
		}
		if combined == "" {
			combined = err.Error()
		}
		return &tool.ToolResult{Success: false, Output: combined}, nil
	}

	output := outStr
	if errStr != "" {
		output = outStr + "\n" + errStr
	}
	if output == "" {
		output = "(command completed with no output)"
	}
	return &tool.ToolResult{Success: true, Output: output}, nil
}
