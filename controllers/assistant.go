package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/coding"
)

// Assistant is a factory function with the prefix and instance
func Assistant() (string, *AssistantController) {
	return "assistant", &AssistantController{}
}

// AssistantController handles AI assistant functionality
type AssistantController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *AssistantController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /assistant", app.Serve("assistant.html", auth.Required))
	http.Handle("GET /repos/{id}/assistant", app.Serve("repo-assistant.html", auth.Required))
	
	http.Handle("POST /assistant/ask", app.ProtectFunc(c.askQuestion, auth.Required))
	http.Handle("POST /repos/{id}/assistant/ask", app.ProtectFunc(c.askRepoQuestion, auth.Required))
	http.Handle("GET /assistant/actions/{id}", app.ProtectFunc(c.getAction, auth.Required))
}

// Handle is called when each request is handled
func (c *AssistantController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// RecentActions returns recent AI actions for display
func (c *AssistantController) RecentActions() ([]*models.Action, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	return models.Actions.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 10", user.ID)
}

// CurrentRepo returns the repository from the URL path for repo-specific assistant
func (c *AssistantController) CurrentRepo() (*coding.GitRepo, error) {
	id := c.Request.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.Coding.GetRepo(id)
	if err != nil {
		return nil, err
	}

	return repo, nil
}

// askQuestion handles general AI questions
func (c *AssistantController) askQuestion(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	question := r.FormValue("question")
	if question == "" {
		c.Render(w, r, "error-message.html", errors.New("question is required"))
		return
	}

	// Create action record
	action := &models.Action{
		Type:     "conversation",
		Title:    c.generateTitle(question),
		Question: question,
		Response: "", // Will be filled by AI response
		Status:   "active",
		UserID:   user.ID,
		RepoID:   "", // General question, no repo context
	}

	_, err = models.Actions.Insert(action)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Simulate AI response (replace with actual AI integration)
	response := c.generateMockResponse(question, "")
	action.Response = response
	action.Status = "completed"

	err = models.Actions.Update(action)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Return the action for display
	c.Render(w, r, "assistant-response.html", action)
}

// askRepoQuestion handles repository-specific AI questions
func (c *AssistantController) askRepoQuestion(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	repo, err := models.Coding.GetRepo(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	question := r.FormValue("question")
	if question == "" {
		c.Render(w, r, "error-message.html", errors.New("question is required"))
		return
	}

	// Create action record
	action := &models.Action{
		Type:     "conversation",
		Title:    c.generateTitle(question),
		Question: question,
		Response: "", // Will be filled by AI response
		Status:   "active",
		UserID:   user.ID,
		RepoID:   repoID,
	}

	_, err = models.Actions.Insert(action)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Simulate AI response with repo context
	response := c.generateMockResponse(question, repo.Name)
	action.Response = response
	action.Status = "completed"

	err = models.Actions.Update(action)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Return the action for display
	c.Render(w, r, "assistant-response.html", action)
}

// getAction returns a specific action for display
func (c *AssistantController) getAction(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	actionID := r.PathValue("id")
	action, err := models.Actions.Get(actionID)
	if err != nil {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}

	// Verify user owns this action
	if action.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(action)
}

// generateTitle creates a short title from the question
func (c *AssistantController) generateTitle(question string) string {
	words := strings.Fields(question)
	if len(words) <= 5 {
		return question
	}
	return strings.Join(words[:5], " ") + "..."
}

// generateMockResponse simulates AI responses (replace with actual AI integration)
func (c *AssistantController) generateMockResponse(question, repoContext string) string {
	responses := map[string]string{
		"hello":        "Hello! I'm Claude, your AI assistant. I can help you with coding questions, explain code, suggest improvements, and more. What would you like to work on?",
		"help":         "I can assist you with various development tasks:\n\n• Code review and suggestions\n• Explaining complex code\n• Debugging assistance\n• Best practices recommendations\n• Documentation generation\n• Testing strategies\n\nJust ask me anything about your code!",
		"code review":  "I'd be happy to help with code review! Please share the specific code you'd like me to review, and I'll provide feedback on:\n\n• Code quality and readability\n• Performance optimizations\n• Security considerations\n• Best practices\n• Potential bugs or issues",
		"bug":          "I can help you debug! To provide the best assistance, please share:\n\n• The specific error message or unexpected behavior\n• The relevant code snippet\n• Steps to reproduce the issue\n• What you expected to happen\n\nI'll analyze the problem and suggest solutions.",
		"performance":  "For performance optimization, I can help with:\n\n• Profiling and identifying bottlenecks\n• Algorithm improvements\n• Database query optimization\n• Caching strategies\n• Memory usage optimization\n\nShare your code and I'll provide specific recommendations!",
	}

	questionLower := strings.ToLower(question)
	
	for keyword, response := range responses {
		if strings.Contains(questionLower, keyword) {
			if repoContext != "" {
				return fmt.Sprintf("**Context: %s repository**\n\n%s", repoContext, response)
			}
			return response
		}
	}

	// Default response
	baseResponse := "Thanks for your question! I'm here to help with coding and development tasks. While I don't have access to external AI services in this demo, in a real implementation I would:\n\n• Analyze your code and repository context\n• Provide detailed explanations and suggestions\n• Help with debugging and optimization\n• Generate code examples and documentation\n\nFor now, I can help you understand how this AI assistant feature works!"

	if repoContext != "" {
		return fmt.Sprintf("**Context: %s repository**\n\n%s\n\nYour question: \"%s\"", repoContext, baseResponse, question)
	}

	return fmt.Sprintf("%s\n\nYour question: \"%s\"", baseResponse, question)
}