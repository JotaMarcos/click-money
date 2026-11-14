// Package migrations embute os arquivos de migração dos dois dialetos, para
// que o binário cmd/migrate não dependa de arquivos em disco.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed postgres/*.sql sqlite/*.sql
var files embed.FS

// Postgres devolve as migrações do PostgreSQL.
func Postgres() fs.FS { return mustSub("postgres") }

// SQLite devolve as migrações do SQLite, usadas em desenvolvimento.
func SQLite() fs.FS { return mustSub("sqlite") }

func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(files, dir)
	if err != nil {
		// Só acontece se o diretório embutido for renomeado sem ajustar este
		// arquivo; é erro de programação detectado na inicialização.
		panic("migrations: " + err.Error())
	}
	return sub
}
