package plugins

import (
	"strings"

	"github.com/copcon/core/storage"
)

// formatTodoState formats a list of todos into a concise system prompt string,
// grouped by status. It prefers the ActiveForm over Content when available.
func formatTodoState(todos []*storage.Todo) string {
	var pending, inProgress, completed, failed, blocked []string

	for _, t := range todos {
		content := t.Content
		if t.ActiveForm != "" {
			content = t.ActiveForm
		}
		switch t.Status {
		case storage.TodoStatusPending:
			pending = append(pending, content)
		case storage.TodoStatusInProgress:
			inProgress = append(inProgress, content)
		case storage.TodoStatusCompleted:
			completed = append(completed, content)
		case storage.TodoStatusFailed:
			failed = append(failed, content)
		case storage.TodoStatusBlocked:
			blocked = append(blocked, content)
		}
	}

	var parts []string
	if len(pending) > 0 {
		parts = append(parts, "pending: "+strings.Join(pending, ", "))
	}
	if len(inProgress) > 0 {
		parts = append(parts, "in_progress: "+strings.Join(inProgress, ", "))
	}
	if len(completed) > 0 {
		parts = append(parts, "completed: "+strings.Join(completed, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(blocked) > 0 {
		parts = append(parts, "blocked: "+strings.Join(blocked, ", "))
	}

	return "Current todo list: [" + strings.Join(parts, ", ") + "]"
}
