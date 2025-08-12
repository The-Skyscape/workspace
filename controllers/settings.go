package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/models"
)

type SettingsController struct {
	application.BaseController
	Request *http.Request
	App     *application.App
}

func Settings() (string, *SettingsController) {
	return "settings", &SettingsController{}
}

func (s *SettingsController) Setup(app *application.App) {
	s.App = app
	s.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Settings pages (admin only)
	http.Handle("GET /settings", app.Serve("settings.html", auth.Required))
	http.Handle("POST /settings", app.ProtectFunc(s.updateSettings, auth.Required))
	
	// AI settings endpoints
	http.Handle("POST /settings/ai/test", app.ProtectFunc(s.testAIProvider, auth.Required))
	http.Handle("POST /settings/vectordb/test", app.ProtectFunc(s.testVectorDB, auth.Required))
	
	// Feature toggles
	http.Handle("POST /settings/features", app.ProtectFunc(s.updateFeatures, auth.Required))
}

func (s *SettingsController) Handle(req *http.Request) application.Controller {
	s.Request = req
	return s
}


// GetSettings returns the current global settings
func (s *SettingsController) GetSettings() (*models.Settings, error) {
	return models.GetSettings()
}

// GetSanitizedSettings returns settings safe for display
func (s *SettingsController) GetSanitizedSettings() map[string]interface{} {
	settings, err := models.GetSettings()
	if err != nil {
		return map[string]interface{}{}
	}
	return settings.SanitizedSettings()
}

// updateSettings handles the main settings form submission
func (s *SettingsController) updateSettings(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Unauthorized",
		})
		return
	}

	// Parse form values
	updates := map[string]interface{}{}

	// AI Provider Settings
	if apiKey := r.FormValue("anthropic_api_key"); apiKey != "" {
		updates["anthropic_api_key"] = apiKey
	}
	if apiKey := r.FormValue("openai_api_key"); apiKey != "" {
		updates["openai_api_key"] = apiKey
	}
	if host := r.FormValue("ollama_host"); host != "" {
		updates["ollama_host"] = host
	}

	// Default AI Settings
	if provider := r.FormValue("default_provider"); provider != "" {
		updates["default_provider"] = provider
	}
	if model := r.FormValue("default_model"); model != "" {
		updates["default_model"] = model
	}
	if temp := r.FormValue("temperature"); temp != "" {
		var temperature float64
		json.Unmarshal([]byte(temp), &temperature)
		updates["temperature"] = temperature
	}
	if tokens := r.FormValue("max_tokens"); tokens != "" {
		var maxTokens int
		json.Unmarshal([]byte(tokens), &maxTokens)
		updates["max_tokens"] = maxTokens
	}

	// Vector Database Settings
	if provider := r.FormValue("vectordb_provider"); provider != "" {
		updates["vectordb_provider"] = provider
	}
	if host := r.FormValue("chroma_host"); host != "" {
		updates["chroma_host"] = host
	}
	if apiKey := r.FormValue("pinecone_api_key"); apiKey != "" {
		updates["pinecone_api_key"] = apiKey
	}
	if env := r.FormValue("pinecone_environment"); env != "" {
		updates["pinecone_environment"] = env
	}
	if host := r.FormValue("weaviate_host"); host != "" {
		updates["weaviate_host"] = host
	}
	if apiKey := r.FormValue("weaviate_api_key"); apiKey != "" {
		updates["weaviate_api_key"] = apiKey
	}

	// Rate Limiting
	if rpm := r.FormValue("ai_requests_per_minute"); rpm != "" {
		var requestsPerMin int
		json.Unmarshal([]byte(rpm), &requestsPerMin)
		updates["ai_requests_per_minute"] = requestsPerMin
	}
	if rpd := r.FormValue("ai_requests_per_day"); rpd != "" {
		var requestsPerDay int
		json.Unmarshal([]byte(rpd), &requestsPerDay)
		updates["ai_requests_per_day"] = requestsPerDay
	}

	// Caching
	if ttl := r.FormValue("cache_ttl_minutes"); ttl != "" {
		var cacheTTL int
		json.Unmarshal([]byte(ttl), &cacheTTL)
		updates["cache_ttl_minutes"] = cacheTTL
	}
	if size := r.FormValue("max_cache_size_mb"); size != "" {
		var cacheSize int64
		json.Unmarshal([]byte(size), &cacheSize)
		updates["max_cache_size"] = cacheSize
	}

	// Update settings
	_, err = models.UpdateSettings(updates, user.Email)
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to update settings: " + err.Error(),
		})
		return
	}

	// Log activity
	models.LogActivity("settings_updated", "Updated global settings", 
		"Administrator updated global settings", user.ID, "", "settings", "")

	// Redirect back to settings page
	s.Redirect(w, r, "/settings")
}

// updateFeatures handles feature toggle updates
func (s *SettingsController) updateFeatures(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Unauthorized",
		})
		return
	}

	updates := map[string]interface{}{
		"enable_ai_agent":        r.FormValue("enable_ai_agent") == "true",
		"enable_code_analysis":   r.FormValue("enable_code_analysis") == "true",
		"enable_auto_summary":    r.FormValue("enable_auto_summary") == "true",
		"enable_semantic_search": r.FormValue("enable_semantic_search") == "true",
		"require_admin_for_ai":   r.FormValue("require_admin_for_ai") == "true",
	}

	_, err = models.UpdateSettings(updates, user.Email)
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to update features: " + err.Error(),
		})
		return
	}

	s.Refresh(w, r)
}

// testAIProvider tests the configured AI provider
func (s *SettingsController) testAIProvider(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}

	settings, err := models.GetSettings()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to get settings"))
		return
	}

	// Test based on provider
	provider := r.FormValue("provider")
	if provider == "" {
		provider = settings.DefaultProvider
	}

	var testResult string
	switch provider {
	case "anthropic":
		if settings.AnthropicAPIKey == "" {
			testResult = "Error: Anthropic API key not configured"
		} else {
			// In real implementation, would test the API
			testResult = "Success: Anthropic API key is configured"
		}
	case "openai":
		if settings.OpenAIAPIKey == "" {
			testResult = "Error: OpenAI API key not configured"
		} else {
			testResult = "Success: OpenAI API key is configured"
		}
	case "ollama":
		if settings.OllamaHost == "" {
			testResult = "Error: Ollama host not configured"
		} else {
			testResult = "Success: Ollama host is configured at " + settings.OllamaHost
		}
	default:
		testResult = "Error: Unknown provider"
	}

	w.Write([]byte(testResult))
}

// testVectorDB tests the configured vector database
func (s *SettingsController) testVectorDB(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}

	settings, err := models.GetSettings()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to get settings"))
		return
	}

	var testResult string
	switch settings.VectorDBProvider {
	case "chroma":
		if settings.ChromaHost == "" {
			testResult = "Error: Chroma host not configured"
		} else {
			testResult = "Success: Chroma is configured at " + settings.ChromaHost
		}
	case "pinecone":
		if settings.PineconeAPIKey == "" || settings.PineconeEnvironment == "" {
			testResult = "Error: Pinecone credentials not configured"
		} else {
			testResult = "Success: Pinecone is configured"
		}
	case "weaviate":
		if settings.WeaviateHost == "" {
			testResult = "Error: Weaviate host not configured"
		} else {
			testResult = "Success: Weaviate is configured at " + settings.WeaviateHost
		}
	default:
		testResult = "Error: No vector database configured"
	}

	w.Write([]byte(testResult))
}

// HasAIEnabled checks if AI features are enabled for display
func (s *SettingsController) HasAIEnabled() bool {
	settings, err := models.GetSettings()
	if err != nil {
		return false
	}
	return settings.IsAIEnabled()
}