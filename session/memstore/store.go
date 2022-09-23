// Package memstore provides an in-RAM (non-persistent) session store
package memstore

import (
	"github.com/quasoft/memstore"

	"github.com/sargassum-world/godest/session"
)

func NewStore(c session.Config) (store *session.Store, backingStore *memstore.MemStore) {
	backingStore = memstore.NewMemStore(c.AuthKey, c.EncryptionKey)
	backingStore.Options = &c.CookieOptions
	backingStore.MaxAge(backingStore.Options.MaxAge)

	return &session.Store{
		Config:       c,
		BackingStore: backingStore,
		Codecs:       backingStore.Codecs,
	}, backingStore
}
