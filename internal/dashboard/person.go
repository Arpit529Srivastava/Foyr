package dashboard

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// Person is who is signed in: the founder, who sees every account, or a customer with a role in some accounts.
type Person struct {
	Founder bool
	UserID  int64
	Email   string
	Roles   map[int64]string // installation id to owner, admin or member
}

// PersonFromUser turns a signed-in dashboard user into a Person.
func PersonFromUser(u store.DashboardUser) Person {
	p := Person{UserID: u.ID, Email: u.Email, Roles: map[int64]string{}}
	for _, a := range u.Accounts {
		p.Roles[a.Installation.ID] = a.Role
	}
	return p
}

// sees reports whether the account is theirs to open.
func (p Person) sees(installationID int64) bool {
	if p.Founder {
		return true
	}
	_, ok := p.Roles[installationID]
	return ok
}

// canChange reports whether they may change things in the account; members only read.
func (p Person) canChange(installationID int64) bool {
	if p.Founder {
		return true
	}
	r := p.Roles[installationID]
	return r == "owner" || r == "admin"
}

// actor is who the audit trail names for their changes.
func (p Person) actor() store.Actor {
	if p.Founder {
		return store.Actor{Type: "system", Login: "founder"}
	}
	return store.Actor{Type: "foyr_user", ID: strconv.FormatInt(p.UserID, 10), Login: p.Email}
}

type personKey struct{}

// personOf is the signed-in person guard stored on the request.
func personOf(r *http.Request) Person {
	p, _ := r.Context().Value(personKey{}).(Person)
	return p
}

func withPerson(r *http.Request, p Person) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), personKey{}, p))
}
