package models

import (
	"encoding/json"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Settings represents global application settings
type Settings struct {
	application.Model
	
	// AI Provider Settings
	AnthropicAPIKey     string `json:"anthropic_api_key"`
	OpenAIAPIKey        string `json:"openai_api_key"`
	OllamaHost          string `json:"ollama_host"`
	
	// Default AI Settings
	DefaultProvider     string `json:"default_provider"` // "anthropic", "openai", "ollama"
	DefaultModel        string `json:"default_model"`
	Temperature         float64 `json:"temperature"`
	MaxTokens           int    `json:"max_tokens"`
	
	// Vector Database Settings
	VectorDBProvider    string `json:"vectordb_provider"` // "chroma", "pinecone", "weaviate"
	ChromaHost          string `json:"chroma_host"`
	PineconeAPIKey      string `json:"pinecone_api_key"`
	PineconeEnvironment string `json:"pinecone_environment"`
	WeaviateHost        string `json:"weaviate_host"`
	WeaviateAPIKey      string `json:"weaviate_api_key"`
	
	// Feature Flags
	EnableAIAgent       bool   `json:"enable_ai_agent"`
	EnableCodeAnalysis  bool   `json:"enable_code_analysis"`
	EnableAutoSummary   bool   `json:"enable_auto_summary"`
	EnableSemanticSearch bool  `json:"enable_semantic_search"`
	
	// Rate Limiting
	AIRequestsPerMinute int    `json:"ai_requests_per_minute"`
	AIRequestsPerDay    int    `json:"ai_requests_per_day"`
	
	// Security
	AllowedDomains      string `json:"allowed_domains"` // Comma-separated list
	RequireAdminForAI   bool   `json:"require_admin_for_ai"`
	
	// Caching
	CacheTTLMinutes     int    `json:"cache_ttl_minutes"`
	MaxCacheSize        int64  `json:"max_cache_size_mb"`
	
	// Metadata
	LastUpdatedBy       string    `json:"last_updated_by"`
	LastUpdatedAt       time.Time `json:"last_updated_at"`
}

// Table returns the database table name
func (*Settings) Table() string { return "settings" }

// GetSettings retrieves the global settings (creates default if not exists)
func GetSettings() (*Settings, error) {
	// Try to get existing settings
	settings, err := GlobalSettings.Get("global")
	if err != nil {
		// Create default settings
		settings = &Settings{
			Model:               DB.NewModel("global"),
			OllamaHost:          "http://localhost:11434",
			DefaultProvider:     "ollama",
			DefaultModel:        "llama2",
			Temperature:         0.7,
			MaxTokens:           2048,
			VectorDBProvider:    "chroma",
			ChromaHost:          "http://localhost:8000",
			EnableAIAgent:       false,
			EnableCodeAnalysis:  true,
			EnableAutoSummary:   false,
			EnableSemanticSearch: false,
			AIRequestsPerMinute: 10,
			AIRequestsPerDay:    1000,
			RequireAdminForAI:   true,
			CacheTTLMinutes:     60,
			MaxCacheSize:        100,
			LastUpdatedAt:       time.Now(),
		}
		
		// Insert default settings
		settings, err = GlobalSettings.Insert(settings)
		if err != nil {
			return nil, err
		}
	}
	
	return settings, nil
}

// UpdateSettings updates the global settings
func UpdateSettings(updates map[string]interface{}, updatedBy string) (*Settings, error) {
	settings, err := GetSettings()
	if err != nil {
		return nil, err
	}
	
	// Marshal updates to JSON and unmarshal into settings
	jsonData, err := json.Marshal(updates)
	if err != nil {
		return nil, err
	}
	
	if err := json.Unmarshal(jsonData, settings); err != nil {
		return nil, err
	}
	
	// Update metadata
	settings.LastUpdatedBy = updatedBy
	settings.LastUpdatedAt = time.Now()
	
	// Save to database
	err = GlobalSettings.Update(settings)
	if err != nil {
		return nil, err
	}
	
	return settings, nil
}

// IsAIEnabled checks if AI features are enabled
func (s *Settings) IsAIEnabled() bool {
	return s.EnableAIAgent && s.HasAIProvider()
}

// HasAIProvider checks if at least one AI provider is configured
func (s *Settings) HasAIProvider() bool {
	return s.AnthropicAPIKey != "" || s.OpenAIAPIKey != "" || s.OllamaHost != ""
}

// HasVectorDB checks if a vector database is configured
func (s *Settings) HasVectorDB() bool {
	switch s.VectorDBProvider {
	case "chroma":
		return s.ChromaHost != ""
	case "pinecone":
		return s.PineconeAPIKey != "" && s.PineconeEnvironment != ""
	case "weaviate":
		return s.WeaviateHost != ""
	default:
		return false
	}
}

// GetAIProviderConfig returns the configuration for the default AI provider
func (s *Settings) GetAIProviderConfig() map[string]interface{} {
	config := map[string]interface{}{
		"provider":    s.DefaultProvider,
		"model":       s.DefaultModel,
		"temperature": s.Temperature,
		"max_tokens":  s.MaxTokens,
	}
	
	switch s.DefaultProvider {
	case "anthropic":
		config["api_key"] = s.AnthropicAPIKey
	case "openai":
		config["api_key"] = s.OpenAIAPIKey
	case "ollama":
		config["host"] = s.OllamaHost
	}
	
	return config
}

// SanitizedSettings returns settings with sensitive fields removed
func (s *Settings) SanitizedSettings() map[string]interface{} {
	return map[string]interface{}{
		"default_provider":      s.DefaultProvider,
		"default_model":         s.DefaultModel,
		"temperature":           s.Temperature,
		"max_tokens":            s.MaxTokens,
		"vectordb_provider":     s.VectorDBProvider,
		"enable_ai_agent":       s.EnableAIAgent,
		"enable_code_analysis":  s.EnableCodeAnalysis,
		"enable_auto_summary":   s.EnableAutoSummary,
		"enable_semantic_search": s.EnableSemanticSearch,
		"ai_requests_per_minute": s.AIRequestsPerMinute,
		"ai_requests_per_day":   s.AIRequestsPerDay,
		"require_admin_for_ai":  s.RequireAdminForAI,
		"cache_ttl_minutes":     s.CacheTTLMinutes,
		"max_cache_size_mb":     s.MaxCacheSize,
		"last_updated_at":       s.LastUpdatedAt,
		"last_updated_by":       s.LastUpdatedBy,
		"has_anthropic":         s.AnthropicAPIKey != "",
		"has_openai":            s.OpenAIAPIKey != "",
		"has_ollama":            s.OllamaHost != "",
		"has_vectordb":          s.HasVectorDB(),
	}
}