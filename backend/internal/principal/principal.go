// Package principal describes who is calling: a signed-in user, a user's personal token,
// or a machine service token. Middleware stores it in the request; services receive it as
// a plain value, so they never depend on the web framework.
package principal

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const contextKey = "actor"

type ServiceToken struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	OrgID      uuid.UUID
	EnvID      uuid.UUID
	EnvSlug    string
	Name       string
	PathPrefix string
	CanWrite   bool
}

type Actor struct {
	UserID uuid.UUID
	Email  string
	ViaPAT bool
	Token  *ServiceToken
	IP     string
}

func (a Actor) IsToken() bool { return a.Token != nil }

func (a Actor) Type() string {
	if a.IsToken() {
		return "token"
	}
	return "user"
}

func (a Actor) ID() string {
	if a.IsToken() {
		return a.Token.ID.String()
	}
	return a.UserID.String()
}

func (a Actor) Label() string {
	if a.IsToken() {
		return "token:" + a.Token.Name
	}
	return a.Email
}

func Set(c *gin.Context, a Actor) { c.Set(contextKey, a) }

func From(c *gin.Context) Actor {
	a, _ := c.Get(contextKey)
	actor, _ := a.(Actor)
	return actor
}
