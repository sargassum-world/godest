package sqlitestore

import (
	"time"

	"zombiezen.com/go/sqlite"
)

// Session

type Session struct {
	ID               string
	Data             string // Data is expected to be a base64-encoded string
	CreationTime     time.Time
	ModificationTime time.Time
	ExpirationTime   time.Time
}

func (s Session) newInsertion() map[string]interface{} {
	return map[string]interface{}{
		"$id":                s.ID,
		"$data":              s.Data,
		"$creation_time":     s.CreationTime.UnixMilli(),
		"$modification_time": s.ModificationTime.UnixMilli(),
		"$expiration_time":   s.ExpirationTime.UnixMilli(),
	}
}

func (s Session) newUpdate() map[string]interface{} {
	return map[string]interface{}{
		"$id":                s.ID,
		"$data":              s.Data,
		"$modification_time": s.ModificationTime.UnixMilli(),
	}
}

func (s Session) newDelete() map[string]interface{} {
	return map[string]interface{}{
		"$id": s.ID,
	}
}

func (s Session) newDeletePastExpiration() map[string]interface{} {
	return map[string]interface{}{
		"$expiration_time_threshold": s.ExpirationTime.UnixMilli(),
	}
}

func newSessionSelection(id string) map[string]interface{} {
	return map[string]interface{}{
		"$id": id,
	}
}

// Sessions

type sessionsSelector struct {
	ids      []string
	sessions map[string]Session
}

func newSessionsSelector() *sessionsSelector {
	return &sessionsSelector{
		ids:      make([]string, 0),
		sessions: make(map[string]Session),
	}
}

func (sel *sessionsSelector) Step(s *sqlite.Stmt) error {
	id := s.GetText("id")
	if _, ok := sel.sessions[id]; !ok {
		sel.sessions[id] = Session{
			ID:               s.GetText("id"),
			Data:             s.GetText("data"),
			CreationTime:     time.UnixMilli(s.GetInt64("creation_time")),
			ModificationTime: time.UnixMilli(s.GetInt64("modification_time")),
			ExpirationTime:   time.UnixMilli(s.GetInt64("expiration_time")),
		}
		if id != "" {
			sel.ids = append(sel.ids, id)
		}
	}
	return nil
}

func (sel *sessionsSelector) Sessions() []Session {
	sessions := make([]Session, len(sel.ids))
	for i, id := range sel.ids {
		sessions[i] = sel.sessions[id]
	}
	return sessions
}
