package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/google/uuid"
)

type AccessToken struct {
	application.Model
	Secret  string
	Expires time.Time
}

func (*AccessToken) Table() string { return "code_access_tokens" }

func NewAccessToken(expires time.Time) (*AccessToken, error) {
	return AccessTokens.Insert(&AccessToken{
		Model:   DB.NewModel(uuid.NewString()),
		Secret:  uuid.NewString(),
		Expires: expires,
	})
}

func GetAccessToken(id, secret string) (*AccessToken, error) {
	token, err := AccessTokens.Get(id)
	if err != nil {
		return nil, err
	}
	if token.Secret != secret || token.Expires.Before(time.Now()) {
		return nil, nil
	}
	return token, nil
}