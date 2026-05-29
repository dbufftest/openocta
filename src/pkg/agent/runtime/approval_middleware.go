package runtime

import (
	"context"
	"fmt"
	"strings"

	octasecurity "github.com/openocta/openocta/pkg/security"
	"github.com/stellarlinkco/agentsdk-go/pkg/middleware"
	"github.com/stellarlinkco/agentsdk-go/pkg/model"
)

func formatApprovalCommand(toolName, target string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "tool"
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return name
	}
	return fmt.Sprintf("%s(%s)", name, target)
}

// approvalQueueMiddleware blocks bash execution until the OpenOcta approval queue allows it
// (agentsdk-go v2 移除了内置 PermissionResolver / ApprovalQueue 挂载点).
func approvalQueueMiddleware(q *octasecurity.ApprovalQueue, blockWait bool) middleware.Middleware {
	return middleware.Funcs{
		Identifier: "openocta-approval-queue",
		OnBeforeTool: func(ctx context.Context, st *middleware.State) error {
			if q == nil || st == nil {
				return nil
			}
			call, ok := st.ToolCall.(model.ToolCall)
			if !ok {
				return nil
			}
			if !strings.EqualFold(strings.TrimSpace(call.Name), "bash") && !strings.EqualFold(strings.TrimSpace(call.Name), "windows_exec_cmd") {
				return nil
			}
			cmd, _ := call.Arguments["command"].(string)
			if strings.TrimSpace(cmd) == "" {
				return nil
			}
			sid, _ := st.Values["session_id"].(string)
			if strings.TrimSpace(sid) == "" {
				return fmt.Errorf("openocta: approval queue requires session_id")
			}
			line := formatApprovalCommand("Bash", strings.TrimSpace(cmd))
			rec, err := q.Request(sid, line, nil)
			if err != nil {
				return err
			}
			if rec.State == octasecurity.ApprovalApproved {
				return nil
			}
			if !blockWait {
				return fmt.Errorf("bash approval required (requestId=%s); approve via gateway", rec.ID)
			}
			resolved, err := q.Wait(ctx, rec.ID)
			if err != nil {
				return err
			}
			switch resolved.State {
			case octasecurity.ApprovalApproved:
				return nil
			case octasecurity.ApprovalDenied:
				reason := strings.TrimSpace(resolved.Reason)
				if reason == "" {
					reason = "denied"
				}
				return fmt.Errorf("bash execution denied: %s", reason)
			default:
				return fmt.Errorf("bash approval left pending")
			}
		},
	}
}
