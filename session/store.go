package session

import (
	"context"
	"net/http"

	"github.com/gorilla/csrf"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
)

type Store struct {
	Config       Config
	BackingStore sessions.Store
	Codecs       []securecookie.Codec
}

func (ss *Store) CSRFOptions() CSRFOptions {
	return ss.Config.CSRFOptions
}

func (ss *Store) New(r *http.Request) (*sessions.Session, error) {
	sess, err := ss.BackingStore.New(r, ss.Config.CookieName)
	return sess, errors.Wrap(err, "couldn't make session")
}

func (ss *Store) Get(r *http.Request) (*sessions.Session, error) {
	sess, err := ss.BackingStore.Get(r, ss.Config.CookieName)
	return sess, errors.Wrap(err, "couldn't get session from request")
}

func (ss *Store) Lookup(id string) (*sessions.Session, error) {
	r, err := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't generate HTTP request to get session")
	}
	encrypted, err := securecookie.EncodeMulti(ss.Config.CookieName, id, ss.Codecs...)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't generate encoded HTTP cookie to get session")
	}
	r.AddCookie(sessions.NewCookie(ss.Config.CookieName, encrypted, &ss.Config.CookieOptions))
	sess, err := ss.BackingStore.Get(r, ss.Config.CookieName)
	return sess, errors.Wrap(err, "couldn't get session without request")
}

func (ss *Store) NewCSRFMiddleware(opts ...csrf.Option) func(http.Handler) http.Handler {
	return NewCSRFMiddleware(ss.Config, opts...)
}
