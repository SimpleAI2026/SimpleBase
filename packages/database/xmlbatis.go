package database

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
)

type (
	XmlBatis struct {
		lock    *sync.RWMutex
		mappers map[string]*XmlParser
		stmts   sync.Map
	}

	BatisStmt struct {
	}
)

func NewXmlBatis() *XmlBatis {
	o := &XmlBatis{}

	o.stmts = sync.Map{}
	o.lock = &sync.RWMutex{}
	o.mappers = make(map[string]*XmlParser)

	return o
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

func (p *XmlBatis) Init(selector string, inputValue map[string]interface{}, fun func(stmt *BatisStmt, queryer *Queryer) error) error {
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
		if stmt, err := p.getStmt(queryer); err != nil {
			return err
		} else {
			if fun != nil {
				fun(stmt, queryer)
			}
		}
	}
	return nil
}

func (p *XmlBatis) QueryObject(result interface{}, selector string, inputValue map[string]interface{}) error {
	return p.exec(selector, inputValue, func(stmt *BatisStmt, queryer *Queryer) error {
		fmt.Println("QueryObject queryer.Sql: ", queryer.Sql)
		return nil
	})
}

func (p *XmlBatis) QueryObjects(result interface{}, selector string, inputValue map[string]interface{}) error {
	return p.exec(selector, inputValue, func(stmt *BatisStmt, queryer *Queryer) error {
		fmt.Println("QueryObjects queryer.Sql: ", queryer.Sql)
		return nil
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
		fmt.Println("update queryer.Sql: ", queryer.Sql)
		return nil
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
	v, found := p.stmts.Load(queryer.Sql)
	if found {
		stmt := v.(*BatisStmt)
		return stmt, nil
	}

	p.lock.Lock()
	v, found = p.stmts.Load(queryer.Sql)
	if found {
		stmt := v.(*BatisStmt)
		p.lock.Unlock()
		return stmt, nil
	}

	var stmt *BatisStmt

	// TODO:

	p.stmts.Store(queryer.Sql, stmt)
	p.lock.Unlock()
	return stmt, nil
}
