package coding

import (
	"github.com/The-Skyscape/devtools/pkg/database"
	"time"

	"github.com/google/uuid"
)

func (*AccessToken) Table() string { return "code_access_tokens" }

type AccessToken struct {
	*Repository

	database.Model
	Secret  string
	Expires time.Time
}

func (r *Repository) NewAccessToken(expires time.Time) (*AccessToken, error) {
	return r.tokens.Insert(&AccessToken{
		Repository: r,
		Model:      r.db.NewModel(uuid.NewString()),
		Secret:     uuid.NewString(),
		Expires:    expires,
	})
}

func (r *Repository) GetAccessToken(id, secret string) (*AccessToken, error) {
	token, err := r.tokens.Get(id)
	if err != nil {
		return nil, err
	}
	if token.Secret != secret || token.Expires.Before(time.Now()) {
		return nil, nil
	}
	return token, nil
}
