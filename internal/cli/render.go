package cli

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
)

func RenderUIMessage(msg conversation.UIMessage) string {
	return RenderUIMessageMarkdown(msg)
}

func RenderUIMessageMarkdown(msg conversation.UIMessage) string {
	switch msg.Type {
	case conversation.UIMessageText:
		return strings.TrimSpace(msg.Content)
	case conversation.UIMessageReasoning:
		return fmt.Sprintf("> Reasoning\n>\n> %s", strings.ReplaceAll(strings.TrimSpace(msg.Content), "\n", "\n> "))
	case conversation.UIMessageTool:
		state := "done"
		if msg.Running != nil && *msg.Running {
			state = "running"
		}
		return fmt.Sprintf("**Tool:** `%s` (%s)", strings.TrimSpace(msg.Name), state)
	case conversation.UIMessageAttachments:
		return fmt.Sprintf("**Attachments:** %d", len(msg.Attachments))
	default:
		return strings.TrimSpace(msg.Content)
	}
}
