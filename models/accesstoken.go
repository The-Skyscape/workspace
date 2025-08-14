package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// AccessToken for repository access
type AccessToken struct {
	application.Model
	RepoID    string
	UserID    string
	Token     string
	ExpiresAt time.Time
}

func (*AccessToken) Table() string { return "access_tokens" }

func init() {
	// Create indexes for access_tokens table
	go func() {
		AccessTokens.Index("RepoID")
		AccessTokens.Index("ExpiresAt")
		AccessTokens.Index("UserID")
	}()
}


// GenerateToken creates a secure random token
func GenerateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateAccessToken creates a new access token
func CreateAccessToken(repoID, userID string, duration time.Duration) (*AccessToken, error) {
	token := &AccessToken{
		Model:     DB.NewModel(""), // Generate a new ID
		RepoID:    repoID,
		UserID:    userID,
		Token:     GenerateToken(),
		ExpiresAt: time.Now().Add(duration),
	}
	return AccessTokens.Insert(token)
}

// GetValidToken retrieves and validates a token
func GetValidToken(token string) (*AccessToken, error) {
	tokens, err := AccessTokens.Search("WHERE Token = ? AND ExpiresAt > ?", token, time.Now())
	if err != nil || len(tokens) == 0 {
		return nil, err
	}
	return tokens[0], nil
}