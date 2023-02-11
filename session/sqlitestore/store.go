// Package sqlitestore store provides a sqlite-backed session store using [database.DB]
package sqlitestore

import (
	"context"
	_ "embed"
	"encoding/base32"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/database"
	"github.com/sargassum-world/godest/handling"
	"github.com/sargassum-world/godest/session"
)

// SqliteStore stores sessions in a godest sqlite-backed [database.DB].
type SqliteStore struct {
	Codecs          []securecookie.Codec
	Options         *sessions.Options
	AbsoluteTimeout time.Duration
	db              *database.DB
}

// NewSqliteStore returns a new SqliteStore.
//
// The db argument should be a [database.DB] with a "sessions_session" table already initialized
// according to the schema defined by the migrations in [NewDomainEmbeds].
//
// See [sessions.NewCookieStore] for a description of the other parameters.
func NewSqliteStore(
	db *database.DB, absoluteTimeout time.Duration, keyPairs ...[]byte,
) *SqliteStore {
	// TODO: also take a parameter for a function to modify a Session with *sssion.Session.Values,
	// which would enable e.g. storing the user identity in a column rather than an encoded string,
	// to enable selecting all sessions fo a user
	// We don't allow specifying the table name programmatically (which would require turning our
	// embedded migrations and queries into Go templates to render into sql), because renaming the
	// table would require its own dedicated migration, and it's too much complexity to deal with that
	// use case.
	const maxAge = 86400 * 30
	store := &SqliteStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: maxAge,
		},
		AbsoluteTimeout: absoluteTimeout,
		db:              db,
	}

	store.MaxAge(store.Options.MaxAge)
	return store
}

// MaxLength restricts the maximum length of new sessions to l.
// If l is 0 there is no limit to the size of a sesson, use with caution.
// The default for a new SqliteStore is 4096 (default for securecookie).
func (ss *SqliteStore) MaxLength(l int) {
	for _, c := range ss.Codecs {
		if codec, ok := c.(*securecookie.SecureCookie); ok {
			codec.MaxLength(l)
		}
	}
}

// MaxAge sets the maximum age for the store and the underlying cookie implementation. Individual
// sessions can be deleted by setting [session.Options.MaxAge] = -1 for that session.
func (ss *SqliteStore) MaxAge(age int) {
	ss.Options.MaxAge = age

	// Set the maxAge for each securecookie instance
	for _, codec := range ss.Codecs {
		if sc, ok := codec.(*securecookie.SecureCookie); ok {
			sc.MaxAge(age)
		}
	}
}

// Get returns a session for the given name after adding it to the registry.
//
// See [sessions.CookieStore.Get].
func (ss *SqliteStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(ss, name)
}

// New creates and returns a session for the given name without adding it to the registry.
//
// See [sessions.CookieStore.New].
func (ss *SqliteStore) New(r *http.Request, name string) (*sessions.Session, error) {
	s := sessions.NewSession(ss, name)
	options := *ss.Options
	s.Options = &options
	s.IsNew = true
	cookie, err := r.Cookie(name)
	if err != nil {
		// Cookie not found, so this is a new session
		return s, nil
	}
	if err = securecookie.DecodeMulti(name, cookie.Value, &s.ID, ss.Codecs...); err != nil {
		// Treat invalid cookie as a new session
		return s, errors.Wrapf(err, "couldn't decode cookie value %s into session id", cookie.Value)
	}

	sess, err := ss.GetSession(r.Context(), s.ID)
	if err != nil {
		// No value found in database, so treat this as a new session
		return s, nil
	}
	if time.Until(sess.ExpirationTime) < 0 {
		return s, errors.Errorf(
			"Session expired on %s, but it is %s now", sess.ExpirationTime, time.Now(),
		)
	}

	if err := securecookie.DecodeMulti(s.Name(), sess.Data, &s.Values, ss.Codecs...); err != nil {
		// Database value couldn't be decoded into session values, so treat this as a new session
		return s, nil
	}
	s.IsNew = false
	return s, nil
}

var base32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Save adds a single session to the response.
//
// If the Options.MaxAge of the session is <= 0 then the session record will be deleted from the
// database. With this process it properly enforces session cookie handling, so no need to trust
// the web browser's cookie management.
func (ss *SqliteStore) Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error {
	// TODO: log all errors in this function, because they don't get bubbled up for some reason!
	// When we exit early with an error, the server may return a 0b respose with 200
	// FIXME: or is there something wrong with how we're using github.com/gorilla/sessions that is
	// suppressing the errors?
	// Delete if max-age is < 0
	if s.Options.MaxAge < 0 {
		if err := ss.DeleteSession(r.Context(), s.ID); err != nil {
			return errors.Wrap(err, "couldn't delete session with max-age <= 0")
		}
		http.SetCookie(w, sessions.NewCookie(s.Name(), "", s.Options))
		return nil
	}

	if s.ID == "" || s.IsNew {
		const idSize = 32
		s.ID = base32Encoding.EncodeToString(securecookie.GenerateRandomKey(idSize))
		sess, err := ss.createdSession(s)
		if err != nil {
			return errors.Wrap(err, "couldn't convert cookie session into record to insert into db")
		}
		if err = ss.AddSession(r.Context(), sess); err != nil {
			return err
		}
	} else {
		sess, err := ss.modifiedSession(s)
		if err != nil {
			return errors.Wrap(err, "couldn't convert cookie session into record to update in db")
		}
		if err := ss.UpdateSession(r.Context(), sess); err != nil {
			return err
		}
	}

	encoded, err := securecookie.EncodeMulti(s.Name(), s.ID, ss.Codecs...)
	if err != nil {
		return errors.Wrap(err, "couldn't encode session id for cookie")
	}
	http.SetCookie(w, sessions.NewCookie(s.Name(), encoded, s.Options))
	return nil
}

func (ss *SqliteStore) createdSession(s *sessions.Session) (sess Session, err error) {
	sess.ID = s.ID
	sess.Data, err = securecookie.EncodeMulti(s.Name(), s.Values, ss.Codecs...)
	if err != nil {
		return Session{}, errors.Wrap(err, "couldn't encode session values")
	}
	sess.CreationTime = time.Now()
	sess.ModificationTime = sess.CreationTime
	sess.ExpirationTime = sess.CreationTime.Add(ss.AbsoluteTimeout)
	return sess, nil
}

func (ss *SqliteStore) modifiedSession(s *sessions.Session) (sess Session, err error) {
	sess.ID = s.ID
	sess.Data, err = securecookie.EncodeMulti(s.Name(), s.Values, ss.Codecs...)
	if err != nil {
		return Session{}, errors.Wrap(err, "couldn't encode session values")
	}
	sess.ModificationTime = time.Now()
	return sess, nil
}

//go:embed queries/insert-session.sql
var rawInsertSessionQuery string
var insertSessionQuery string = strings.TrimSpace(rawInsertSessionQuery)

func (ss *SqliteStore) AddSession(ctx context.Context, sess Session) (err error) {
	err = ss.db.ExecuteInsertion(ctx, insertSessionQuery, sess.newInsertion())
	if err != nil {
		return errors.Wrapf(err, "couldn't add session")
	}
	return nil
}

//go:embed queries/update-session.sql
var rawUpdateSessionQuery string
var updateSessionQuery string = strings.TrimSpace(rawUpdateSessionQuery)

func (ss *SqliteStore) UpdateSession(ctx context.Context, sess Session) (err error) {
	return errors.Wrapf(
		ss.db.ExecuteUpdate(ctx, updateSessionQuery, sess.newUpdate()),
		"couldn't update session with id %s", sess.ID,
	)
}

//go:embed queries/delete-session.sql
var rawDeleteSessionQuery string
var deleteSessionQuery string = strings.TrimSpace(rawDeleteSessionQuery)

func (ss *SqliteStore) DeleteSession(ctx context.Context, id string) (err error) {
	return errors.Wrapf(
		ss.db.ExecuteDelete(ctx, deleteSessionQuery, Session{ID: id}.newDelete()),
		"couldn't delete session with id %s", id,
	)
}

//go:embed queries/delete-sessions-past-creation-timeout.sql
var rawDeleteOldCreatedSessionsQuery string
var deleteOldCreatedSessionsQuery string = strings.TrimSpace(rawDeleteOldCreatedSessionsQuery)

func (ss *SqliteStore) DeleteOldCreatedSessions(
	ctx context.Context, timeout time.Duration, threshold time.Time,
) (err error) {
	return errors.Wrapf(
		ss.db.ExecuteDelete(
			ctx, deleteOldCreatedSessionsQuery, Session{
				ExpirationTime: threshold,
			}.newDeletePastCreationTimeout(timeout),
		),
		"couldn't delete sessions which expired before %s", threshold,
	)
}

//go:embed queries/delete-sessions-past-expiration.sql
var rawDeleteExpiredSessionsQuery string
var deleteExpiredSessionsQuery string = strings.TrimSpace(rawDeleteExpiredSessionsQuery)

func (ss *SqliteStore) DeleteExpiredSessions(ctx context.Context, threshold time.Time) (err error) {
	return errors.Wrapf(
		ss.db.ExecuteDelete(
			ctx, deleteExpiredSessionsQuery, Session{ExpirationTime: threshold}.newDeletePastExpiration(),
		),
		"couldn't delete sessions which expired before %s", threshold,
	)
}

func (ss *SqliteStore) Cleanup(ctx context.Context) (done bool, err error) {
	// This will delete any session based on its absolute timeout at the time of creation, even if the
	// absolute timeout later increased such that the session's age is greater than the current
	// timeout.
	if err := ss.DeleteExpiredSessions(ctx, time.Now()); err != nil {
		return false, errors.Wrap(err, "couldn't perform periodic deletion of expired sessions")
	}
	// This will delete any session based on the current absolute timeout, even if the absolute
	// timeout was decreased such that current timeout is less than the session's expiration time in
	// the database.
	if err := ss.DeleteOldCreatedSessions(ctx, ss.AbsoluteTimeout, time.Now()); err != nil {
		return false, errors.Wrap(
			err, "couldn't perform periodic deletion of sessions created a long time ago",
		)
	}
	return false, nil
}

func (ss *SqliteStore) PeriodicallyCleanup(
	ctx context.Context, interval time.Duration,
) error {
	return handling.Repeat(ctx, interval, func() (done bool, err error) {
		return ss.Cleanup(ctx)
	})
}

//go:embed queries/select-session.sql
var rawSelectSessionQuery string
var selectSessionQuery string = strings.TrimSpace(rawSelectSessionQuery)

func (ss *SqliteStore) GetSession(ctx context.Context, id string) (sess Session, err error) {
	sel := newSessionsSelector()
	if err = ss.db.ExecuteSelection(
		ctx, selectSessionQuery, newSessionSelection(id), sel.Step,
	); err != nil {
		return Session{}, errors.Wrapf(err, "couldn't get session with id %s", id)
	}
	sessions := sel.Sessions()
	if len(sessions) == 0 {
		return Session{}, errors.Errorf("couldn't get non-existent session with id %s", id)
	}
	return sessions[0], nil
}

func NewStore(db *database.DB, c session.Config) (store *session.Store, backingStore *SqliteStore) {
	backingStore = NewSqliteStore(db, c.Timeouts.Absolute, c.AuthKey, c.EncryptionKey)
	backingStore.Options = &c.CookieOptions
	backingStore.MaxAge(backingStore.Options.MaxAge)

	return &session.Store{
		Config:       c,
		BackingStore: backingStore,
		Codecs:       backingStore.Codecs,
	}, backingStore
}
