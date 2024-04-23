package main

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/dolthub/go-mysql-server/sql"
)

type (
	// Database is an implementation of a go-mysql-server database
	// backed by a SQLite database.
	Database struct {
		name    string
		options DatabaseOptions
	}

	// DatabaseOptions are options for managing the SQLite backend
	DatabaseOptions struct {
		// PreventInserts will block table insertions
		PreventInserts bool
		// PreventCreateTable will block table creation
		PreventCreateTable bool
	}

	provider struct {
		mut       sync.RWMutex
		databases map[string]*Database
	}
)

var _ sql.DatabaseProvider = &provider{}
var _ sql.MutableDatabaseProvider = &provider{}

func NewProvider(dbs ...sql.Database) *provider {
	databases := make(map[string]*Database, len(dbs))
	for _, db := range dbs {
		if v, ok := db.(*Database); !ok {
			continue
		} else {
			databases[strings.ToLower(db.Name())] = v
		}
	}

	return &provider{
		databases: databases,
	}
}

func (p *provider) Database(ctx *sql.Context, name string) (sql.Database, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()
	name = strings.ToLower(name)

	if db, ok := p.databases[name]; !ok {
		return nil, sql.ErrDatabaseNotFound.New()
	} else {
		return db, nil
	}
}

func (p *provider) HasDatabase(ctx *sql.Context, name string) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()
	name = strings.ToLower(name)

	_, ok := p.databases[name]
	return ok
}

func (p *provider) AllDatabases(ctx *sql.Context) []sql.Database {
	p.mut.RLock()
	defer p.mut.RUnlock()

	all := make([]sql.Database, len(p.databases))
	var i int
	for _, db := range p.databases {
		all[i] = db
		i++
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})

	return all
}

func (p *provider) CreateDatabase(ctx *sql.Context, name string) error {
	return errors.New("Not support") // TODO: fix
}

func (p *provider) DropDatabase(ctx *sql.Context, name string) error {
	return errors.New("Not support") // TODO: fix
}
