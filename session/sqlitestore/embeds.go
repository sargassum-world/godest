package sqlitestore

import (
	"embed"
	"io/fs"

	"github.com/sargassum-world/godest/database"
)

// Migrations

var (
	//go:embed migrations/*
	migrationsEFS   embed.FS
	migrationsFS, _ = fs.Sub(migrationsEFS, "migrations")
)

var MigrationFiles []string = []string{
	"1-initialize-schema-v0.3.0",
}

// Embeds

func NewDomainEmbeds() database.DomainEmbeds {
	return database.DomainEmbeds{
		MigrationsFS: migrationsFS,
	}
}
