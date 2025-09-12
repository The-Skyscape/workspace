package tools

import (
	"fmt"
	"strings"
	"workspace/models"
)

// TodoListTool lists todos for the current conversation
type TodoListTool struct{}

func (t *TodoListTool) Name() string {
	return "todo_list"
}

func (t *TodoListTool) Description() string {
	return "List all todos/tasks for the current conversation. No params required."
}

func (t *TodoListTool) ValidateParams(params map[string]any) error {
	// No params required - conversation ID comes from context
	return nil
}

func (t *TodoListTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *TodoListTool) Execute(params map[string]any, userID string) (string, error) {
	// Get conversation ID from params (will be injected by controller)
	conversationIDVal, exists := params["_conversation_id"]
	if !exists {
		return "", fmt.Errorf("conversation ID not provided")
	}

	conversationID, ok := conversationIDVal.(string)
	if !ok || conversationID == "" {
		return "", fmt.Errorf("invalid conversation ID")
	}
	// Get todos for the conversation
	todos, err := models.GetTodosByConversation(conversationID)
	if err != nil {
		return "", fmt.Errorf("failed to get todos: %w", err)
	}

	if len(todos) == 0 {
		return "No todos yet. Use todo_update to add tasks.", nil
	}

	// Format todos
	var result strings.Builder
	result.WriteString("## Task List\n\n")

	// Group by status
	var pending, inProgress, completed []*models.Todo
	for _, todo := range todos {
		switch todo.Status {
		case models.TodoStatusInProgress:
			inProgress = append(inProgress, todo)
		case models.TodoStatusCompleted:
			completed = append(completed, todo)
		default:
			pending = append(pending, todo)
		}
	}

	// Show in progress first
	if len(inProgress) > 0 {
		result.WriteString("### 🔵 In Progress\n")
		for _, todo := range inProgress {
			result.WriteString(fmt.Sprintf("- %s\n", todo.Content))
		}
		result.WriteString("\n")
	}

	// Then pending
	if len(pending) > 0 {
		result.WriteString("### ⬜ Pending\n")
		for _, todo := range pending {
			result.WriteString(fmt.Sprintf("- %s\n", todo.Content))
		}
		result.WriteString("\n")
	}

	// Finally completed
	if len(completed) > 0 {
		result.WriteString("### ✅ Completed\n")
		for _, todo := range completed {
			result.WriteString(fmt.Sprintf("- %s\n", todo.Content))
		}
		result.WriteString("\n")
	}

	// Add summary
	total := len(todos)
	completedCount := len(completed)
	percentage := 0
	if total > 0 {
		percentage = (completedCount * 100) / total
	}

	result.WriteString(fmt.Sprintf("\n**Progress:** %d/%d tasks completed (%d%%)",
		completedCount, total, percentage))

	return result.String(), nil
}

// TodoUpdateTool manages todos for the conversation
type TodoUpdateTool struct{}

func (t *TodoUpdateTool) Name() string {
	return "todo_update"
}

func (t *TodoUpdateTool) Description() string {
	return "Add, update, or remove todos. Required: action (add/update/remove/clear). For add: content. For update/remove: todo_id or content. For update: new_status or new_content."
}

func (t *TodoUpdateTool) ValidateParams(params map[string]any) error {
	action, exists := params["action"]
	if !exists {
		return fmt.Errorf("action is required (add/update/remove/clear)")
	}

	actionStr, ok := action.(string)
	if !ok {
		return fmt.Errorf("action must be a string")
	}

	switch actionStr {
	case "add":
		if _, exists := params["content"]; !exists {
			return fmt.Errorf("content is required for add action")
		}
	case "update":
		// Need either todo_id or content to identify the todo
		if _, hasID := params["todo_id"]; !hasID {
			if _, hasContent := params["content"]; !hasContent {
				return fmt.Errorf("todo_id or content required for update action")
			}
		}
		// Need either new_status or new_content
		if _, hasStatus := params["new_status"]; !hasStatus {
			if _, hasContent := params["new_content"]; !hasContent {
				return fmt.Errorf("new_status or new_content required for update action")
			}
		}
	case "remove":
		// Need either todo_id or content to identify the todo
		if _, hasID := params["todo_id"]; !hasID {
			if _, hasContent := params["content"]; !hasContent {
				return fmt.Errorf("todo_id or content required for remove action")
			}
		}
	case "clear":
		// No additional params needed
	default:
		return fmt.Errorf("invalid action: %s (must be add/update/remove/clear)", actionStr)
	}

	return nil
}

func (t *TodoUpdateTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"add", "update", "remove", "clear"},
				"description": "The action to perform",
				"required":    true,
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Todo content (for add) or to identify todo (for update/remove)",
			},
			"todo_id": map[string]any{
				"type":        "string",
				"description": "ID of the todo to update/remove",
			},
			"new_status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed"},
				"description": "New status for update action",
			},
			"new_content": map[string]any{
				"type":        "string",
				"description": "New content for update action",
			},
		},
		"required": []string{"action"},
	}
}

func (t *TodoUpdateTool) Execute(params map[string]any, userID string) (string, error) {
	// Get conversation ID from params (will be injected by controller)
	conversationIDVal, exists := params["_conversation_id"]
	if !exists {
		return "", fmt.Errorf("conversation ID not provided")
	}

	conversationID, ok := conversationIDVal.(string)
	if !ok || conversationID == "" {
		return "", fmt.Errorf("invalid conversation ID")
	}
	action := params["action"].(string)

	switch action {
	case "add":
		content := params["content"].(string)

		// Create new todo
		todo := &models.Todo{
			ConversationID: conversationID,
			Content:        content,
			Status:         models.TodoStatusPending,
			Position:       models.GetNextTodoPosition(conversationID),
		}

		todo, err := models.Todos.Insert(todo)
		if err != nil {
			return "", fmt.Errorf("failed to add todo: %w", err)
		}

		return fmt.Sprintf("✅ Added todo: %s", content), nil

	case "update":
		// Find the todo
		var todo *models.Todo

		if idVal, exists := params["todo_id"]; exists {
			// Find by ID
			id, ok := idVal.(string)
			if !ok || id == "" {
				return "", fmt.Errorf("invalid todo ID")
			}

			var err error
			todo, err = models.Todos.Get(id)
			if err != nil {
				return "", fmt.Errorf("todo not found with ID %s", id)
			}
		} else if contentVal, exists := params["content"]; exists {
			// Find by content
			content := contentVal.(string)
			todos, err := models.GetTodosByConversation(conversationID)
			if err != nil {
				return "", fmt.Errorf("failed to get todos: %w", err)
			}

			for _, t := range todos {
				if strings.Contains(strings.ToLower(t.Content), strings.ToLower(content)) {
					todo = t
					break
				}
			}

			if todo == nil {
				return "", fmt.Errorf("todo not found with content matching: %s", content)
			}
		}

		// Update the todo
		updated := false
		if statusVal, exists := params["new_status"]; exists {
			status := statusVal.(string)
			todo.Status = status
			updated = true
		}

		if contentVal, exists := params["new_content"]; exists {
			content := contentVal.(string)
			todo.Content = content
			updated = true
		}

		if updated {
			err := models.Todos.Update(todo)
			if err != nil {
				return "", fmt.Errorf("failed to update todo: %w", err)
			}

			statusIcon := "⬜"
			if todo.Status == models.TodoStatusInProgress {
				statusIcon = "🔵"
			} else if todo.Status == models.TodoStatusCompleted {
				statusIcon = "✅"
			}

			return fmt.Sprintf("%s Updated: %s (%s)", statusIcon, todo.Content, todo.Status), nil
		}

		return "No updates made", nil

	case "remove":
		// Find the todo
		var todo *models.Todo

		if idVal, exists := params["todo_id"]; exists {
			// Find by ID
			id, ok := idVal.(string)
			if !ok || id == "" {
				return "", fmt.Errorf("invalid todo ID")
			}

			var err error
			todo, err = models.Todos.Get(id)
			if err != nil {
				return "", fmt.Errorf("todo not found with ID %s", id)
			}
		} else if contentVal, exists := params["content"]; exists {
			// Find by content
			content := contentVal.(string)
			todos, err := models.GetTodosByConversation(conversationID)
			if err != nil {
				return "", fmt.Errorf("failed to get todos: %w", err)
			}

			for _, t := range todos {
				if strings.Contains(strings.ToLower(t.Content), strings.ToLower(content)) {
					todo = t
					break
				}
			}

			if todo == nil {
				return "", fmt.Errorf("todo not found with content matching: %s", content)
			}
		}

		// Remove the todo
		err := models.Todos.Delete(todo)
		if err != nil {
			return "", fmt.Errorf("failed to remove todo: %w", err)
		}

		return fmt.Sprintf("🗑️ Removed todo: %s", todo.Content), nil

	case "clear":
		// Remove all todos for the conversation
		todos, err := models.GetTodosByConversation(conversationID)
		if err != nil {
			return "", fmt.Errorf("failed to get todos: %w", err)
		}

		for _, todo := range todos {
			err := models.Todos.Delete(todo)
			if err != nil {
				// Continue even if one fails
				continue
			}
		}

		return "🧹 Cleared all todos", nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}
