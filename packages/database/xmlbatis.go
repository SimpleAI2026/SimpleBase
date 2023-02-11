package database

import (
	"database/sql"
	"errors"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type (
	XmlBatis struct {
		lock    *sync.RWMutex
		db      *sqlx.DB
		mappers map[string]*XmlParser
		stmts   sync.Map
	}

	BatisStmt struct {
		sqlStmt *sqlx.NamedStmt
	}
)

func NewXmlBatis(config DbConfig) (*XmlBatis, error) {
	o := &XmlBatis{}
	o.stmts = sync.Map{}
	o.lock = &sync.RWMutex{}
	o.mappers = make(map[string]*XmlParser)

	if o.db = sqlx.MustOpen(config.Driver, config.Dsn).Unsafe(); o.db == nil {
		return nil, errors.New("open " + config.Dsn + " failed!")
	}
	o.db.SetMaxIdleConns(config.MaxIdleConnections)
	o.db.SetMaxOpenConns(config.MaxOpenConnections)
	o.db.SetConnMaxIdleTime(time.Duration(config.MaxIdleConnections) * time.Second)
	o.db.SetConnMaxLifetime(time.Duration(config.MaxLifeTime) * time.Second)

	return o, nil
}

func (p *XmlBatis) LoadFromBytes(name string, bytes []byte) error {
	parser := &XmlParser{}
	if err := parser.LoadFromBytes(bytes); err != nil {
		return err
	}
	p.mappers[name] = parser
	return nil
}

func (p *XmlBatis) LoadFromStr(name, str string) error {
	return p.LoadFromBytes(name, []byte(str))
}

func (p *XmlBatis) LoadFromFile(name, filename string) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	return p.LoadFromBytes(name, b)
}

func (p *XmlBatis) PreExec(selector string, inputValue map[string]interface{}) error {
	s, err := p.getSelector(selector)
	if err != nil {
		return err
	}

	parser, ok := p.mappers[s.Name]
	if !ok {
		return errors.New("XML file \"" + s.Name + "\" is not exists!")
	}

	queryerList, err := parser.QueryElements(s.Id, inputValue)
	if err != nil {
		return err
	}

	for _, queryer := range queryerList {
		_, err := p.db.Exec(queryer.Sql)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *XmlBatis) QueryObject(result interface{}, selector string, inputValue map[string]interface{}) error {
	return p.exec(selector, inputValue, func(stmt *BatisStmt, queryer *Queryer) error {
		if stmt == nil {
			return p.db.Get(result, queryer.Sql)
		}
		return stmt.sqlStmt.Get(result, queryer.Value)
	})
}

func (p *XmlBatis) QueryObjects(result interface{}, selector string, inputValue map[string]interface{}) error {
	return p.exec(selector, inputValue, func(stmt *BatisStmt, queryer *Queryer) error {
		if stmt == nil {
			return p.db.Select(result, queryer.Sql)
		}
		return stmt.sqlStmt.Select(result, queryer.Value)
	})
}

func (p *XmlBatis) Insert(selector string, inputValue map[string]interface{}) (int64, error) {
	return p.update(selector, inputValue)
}

func (p *XmlBatis) Update(selector string, inputValue map[string]interface{}) (int64, error) {
	return p.update(selector, inputValue)
}

func (p *XmlBatis) Delete(selector string, inputValue map[string]interface{}) (int64, error) {
	return p.update(selector, inputValue)
}

func (p *XmlBatis) exec(selector string, inputValue map[string]interface{}, fun func(stmt *BatisStmt, queryer *Queryer) error) error {
	s, err := p.getSelector(selector)
	if err != nil {
		return err
	}
	parser, ok := p.mappers[s.Name]
	if !ok {
		return errors.New("XML file \"" + s.Name + "\" is not exists!")
	}
	queryer, err := parser.Query(s.Id, inputValue)
	if err != nil {
		return err
	}
	if queryer.StatementType == "STATEMENT" {
		return fun(nil, queryer)
	}
	if stmt, err := p.getStmt(queryer); err != nil {
		return err
	} else {
		return fun(stmt, queryer)
	}
}

func (p *XmlBatis) update(selector string, inputValue map[string]interface{}) (int64, error) {
	var n int64 = 0
	err := p.exec(selector, inputValue, func(stmt *BatisStmt, queryer *Queryer) error {
		var result sql.Result
		var err error

		if stmt == nil {
			result, err = p.db.Exec(queryer.Sql)
		} else {
			result, err = stmt.sqlStmt.Exec(queryer.Value)
		}

		if err != nil {
			return err
		}

		if strings.ToUpper(queryer.Sql[0:6]) == "INSERT" {
			n, err = result.LastInsertId()
		} else {
			n, err = result.RowsAffected()
		}

		return err
	})
	return n, err
}

func (p *XmlBatis) getSelector(selector string) (*selectorEntity, error) {
	arr := strings.Split(selector, ".")
	if len(arr) != 2 {
		return nil, errors.New("Selector \"" + selector + "\" is not exists!")
	}
	return &selectorEntity{Name: arr[0], Id: arr[1]}, nil
}

func (p *XmlBatis) getStmt(queryer *Queryer) (*BatisStmt, error) {
	if v, found := p.stmts.Load(queryer.Sql); found {
		stmt := v.(*BatisStmt)
		return stmt, nil
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if v, found := p.stmts.Load(queryer.Sql); found {
		stmt := v.(*BatisStmt)
		return stmt, nil
	}

	stmt := new(BatisStmt)
	if sqlStmt, err := p.db.PrepareNamed(queryer.Sql); err != nil {
		return nil, err
	} else {
		stmt.sqlStmt = sqlStmt
	}

	p.stmts.Store(queryer.Sql, stmt)
	return stmt, nil
}
