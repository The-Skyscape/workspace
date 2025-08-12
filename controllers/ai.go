package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"workspace/agents"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// AI controller handles AI-related operations
type AIController struct {
	application.BaseController
	Request *http.Request
}

// AI is the factory function for the AI controller
func AI() (string, *AIController) {
	return "ai", &AIController{}
}

// Setup registers AI routes
func (a *AIController) Setup(app *application.App) {
	a.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// AI Chat interface
	http.Handle("GET /repos/{id}/ai", app.Serve("repo-ai.html", auth.Required))
	
	// AI API endpoints
	http.Handle("POST /repos/{id}/ai/chat", app.ProtectFunc(a.chat, auth.Required))
	http.Handle("POST /repos/{id}/ai/analyze", app.ProtectFunc(a.analyzeCode, auth.Required))
	http.Handle("POST /repos/{id}/ai/summarize", app.ProtectFunc(a.summarizeRepo, auth.Required))
	http.Handle("POST /repos/{id}/ai/explain", app.ProtectFunc(a.explainCode, auth.Required))
	http.Handle("POST /repos/{id}/ai/review", app.ProtectFunc(a.reviewCode, auth.Required))
	http.Handle("POST /repos/{id}/ai/generate-tests", app.ProtectFunc(a.generateTests, auth.Required))
	http.Handle("POST /repos/{id}/ai/generate-docs", app.ProtectFunc(a.generateDocs, auth.Required))
	http.Handle("GET /ai/models", app.ProtectFunc(a.listModels, auth.Required))
	
	// Proxy handler for direct Ollama API access
	http.Handle("/ai/api/", a.proxyToOllama())
}

// Handle prepares the controller for each request
func (a *AIController) Handle(req *http.Request) application.Controller {
	a.Request = req
	return a
}

// IsAvailable checks if AI service is available
func (a *AIController) IsAvailable() bool {
	return services.AI.IsAvailable()
}

// chat handles chat requests with repository context
func (a *AIController) chat(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderChatError(w, r, "Authentication required")
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		a.renderChatError(w, r, "Access denied")
		return
	}

	// Get the message from form data (HTMX sends form data, not JSON)
	message := r.FormValue("message")
	// selectedModel := r.FormValue("model") // TODO: Use for model selection
	
	if message == "" {
		a.renderChatError(w, r, "No message provided")
		return
	}

	// Use the developer agent for enhanced capabilities
	agent := agents.NewDeveloperAgent(repoID)
	
	// TODO: Pass selectedModel to agent if not "auto"
	
	// For now, just process with empty context (can enhance later with session storage)
	context := []map[string]string{}
	
	// Process with agent
	response, err := agent.ProcessQuery(message, context)
	if err != nil {
		a.renderChatError(w, r, fmt.Sprintf("AI error: %v", err))
		return
	}

	// Render both messages as a single HTML response
	// HTMX expects a single response that contains all new content
	var responseHTML strings.Builder
	
	// User message
	responseHTML.WriteString(`<div class="chat chat-end">`)
	responseHTML.WriteString(`<div class="chat-bubble chat-bubble-accent">`)
	responseHTML.WriteString(message)
	responseHTML.WriteString(`</div></div>`)
	
	// Assistant response
	responseHTML.WriteString(`<div class="chat chat-start">`)
	responseHTML.WriteString(`<div class="chat-bubble chat-bubble-primary">`)
	responseHTML.WriteString(response)
	responseHTML.WriteString(`</div></div>`)
	
	// Send the combined HTML
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(responseHTML.String()))
}

// analyzeCode analyzes code from the repository
func (a *AIController) analyzeCode(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Parse request
	var req struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.renderError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Analyze code
	analysis, err := services.AI.AnalyzeCode(req.FilePath, req.Content)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"analysis": analysis,
	})
}

// summarizeRepo generates a repository summary
func (a *AIController) summarizeRepo(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Get repository for context
	repo, err := models.GetRepositoryByID(repoID)
	if err != nil {
		a.renderError(w, r, "Repository not found", http.StatusNotFound)
		return
	}
	
	// Generate summary
	summary, err := services.AI.SummarizeRepository(repoID, repo.Name, repo.Description)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Summary failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"summary": summary,
	})
}

// explainCode explains what code does
func (a *AIController) explainCode(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Parse request
	var req struct {
		Code     string `json:"code"`
		Language string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.renderError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Explain code
	explanation, err := services.AI.ExplainCode(req.Code, req.Language)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Explanation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"explanation": explanation,
	})
}

// reviewCode performs AI code review
func (a *AIController) reviewCode(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Parse request
	var req struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.renderError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Review code
	review, err := services.AI.ReviewCode(req.FilePath, req.Content)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Review failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"review": review,
	})
}

// generateTests generates test cases for code
func (a *AIController) generateTests(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Parse request
	var req struct {
		Code     string `json:"code"`
		Language string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.renderError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Generate tests
	tests, err := services.AI.GenerateTests(req.Code, req.Language)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Test generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"tests": tests,
	})
}

// generateDocs generates documentation for code
func (a *AIController) generateDocs(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	
	// Check repository access
	auth := a.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		a.renderError(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		a.renderError(w, r, "Access denied", http.StatusForbidden)
		return
	}

	// Parse request
	var req struct {
		Code     string `json:"code"`
		Language string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.renderError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Generate documentation
	docs, err := services.AI.GenerateDocumentation(req.Code, req.Language)
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Documentation generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"documentation": docs,
	})
}

// listModels returns available AI models
func (a *AIController) listModels(w http.ResponseWriter, r *http.Request) {
	models, err := services.AI.GetAvailableModels()
	if err != nil {
		a.renderError(w, r, fmt.Sprintf("Failed to list models: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models": models,
	})
}


// proxyToOllama creates a reverse proxy to the Ollama service
func (a *AIController) proxyToOllama() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication
		auth := a.App.Use("auth").(*authentication.Controller)
		_, _, err := auth.Authenticate(r)
		if err != nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Check if Ollama is running
		if !services.Ollama.IsRunning() {
			http.Error(w, "AI service is not available", http.StatusServiceUnavailable)
			return
		}

		// Create reverse proxy
		target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", services.Ollama.GetPort()))
		proxy := httputil.NewSingleHostReverseProxy(target)

		// Update the request
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/ai")
		r.URL.Host = target.Host
		r.URL.Scheme = target.Scheme
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Host = target.Host

		// Serve the request
		proxy.ServeHTTP(w, r)
	})
}

// renderError renders an error response
func (a *AIController) renderError(w http.ResponseWriter, r *http.Request, message string, status int) {
	// Check if this is an API request
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{
			"error": message,
		})
	} else {
		w.WriteHeader(status)
		a.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": message,
		})
	}
}

// renderChatError renders an error message in chat format
func (a *AIController) renderChatError(w http.ResponseWriter, r *http.Request, message string) {
	// Return error as HTML for HTMX
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div class="chat chat-start"><div class="chat-bubble chat-bubble-error">` + message + `</div></div>`))
}

// GetChatHistory returns chat history for a repository (placeholder for future implementation)
func (a *AIController) GetChatHistory(repoID string) []map[string]string {
	// TODO: Implement chat history storage and retrieval
	return []map[string]string{}
}