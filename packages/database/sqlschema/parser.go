package gosqlparser

import (
	"fmt"
	"strings"
)

type parser struct {
	lexer *lexer

	statement Statement
	err       error
}

// Parse parses the input and returns the statement object or an error.
func Parse(input string) (Statement, error) {
	p := &parser{newLexer(input), nil, nil}

	go p.lexer.run()
	p.run()

	return p.statement, p.err
}

// EngineType
type EngineType int

const (
	// EngineDefault means that engine is not specified and default one can be used.
	EngineDefault EngineType = iota
	// EngineLSM defines LSM-tree engine.
	EngineLSM EngineType = iota
	// EngineBPTree defines B+ tree engine.
	EngineBPTree
)

// ColumnType for predefined column types.
type ColumnType int

const (
	// TypeInteger is an integer type of the column.
	TypeInteger ColumnType = iota
	// TypeString is a string type of the column.
	TypeString
)

// Name returns the name of the type represented as a string.
func (t ColumnType) Name() string {
	switch t {
	case TypeInteger:
		return "integer"
	case TypeString:
		return "string"
	default:
		return "unknown"
	}
}

// Operator for prefined operator types.
type Operator int

const (
	// OperatorEquals represents "=="
	OperatorEquals Operator = iota
	// OperatorLogicalAnd represents "AND"
	OperatorLogicalAnd
)

type StatementType int

const (
	// StatementSelect for SELECT query
	StatementSelect StatementType = iota
	// StatementUpdate for UPDATE query
	StatementUpdate
	// StatementInsert for INSERT query
	StatementInsert
	// StatementDelete for DELETE query
	StatementDelete
	// StatementCreateTable for CREATE TABLE query
	StatementCreateTable
	// StatementDropTable for DROP TABLE query
	StatementDropTable
)

// Statement represents parsed SQL statement. Can be one of
// Select, Insert, Update, Delete, CreateTable or DropTable.
type Statement interface {
	// GetType returns the statement type.
	GetType() StatementType
}

// GetType returns the statement type.
func (*Select) GetType() StatementType { return StatementSelect }

// GetType returns the statement type.
func (*Insert) GetType() StatementType { return StatementInsert }

// GetType returns the statement type.
func (*Update) GetType() StatementType { return StatementUpdate }

// GetType returns the statement type.
func (*Delete) GetType() StatementType { return StatementDelete }

// GetType returns the statement type.
func (*CreateTable) GetType() StatementType { return StatementCreateTable }

// GetType returns the statement type.
func (*DropTable) GetType() StatementType { return StatementDropTable }

// Insert represents INSERT query.
type Insert struct {
	Table   string
	Columns []string
	Values  []string
}

// Update represents UPDATE query.
type Update struct {
	Table   string
	Columns []string
	Values  []string
	Where   *Where
}

// Delete represents DELETE query.
//
//	DELETE FROM Table
//	WHERE
//		...
type Delete struct {
	Table string
	Where *Where
}

// CreateTable represents CREATE TABLE statement.
type CreateTable struct {
	Name    string
	Columns []ColumnDefinition
	Engine  EngineType
}

// ColumnDefinition represents the column definition for CREATE TABLE query.
type ColumnDefinition struct {
	Name string
	Type ColumnType
}

// DropTable represents DROP TABLE statement.
type DropTable struct {
	Table string
}

// Select represents parsed SELECT SQL statement.
type Select struct {
	Table   string
	Columns []string
	Where   *Where
	Limit   string
}

// Where represent conditional expressions.
type Where struct {
	Expr Expr
}

// Expr represents expression that can be used in WHERE statement.
type Expr interface {
	i()
}

func (ExprIdentifier) i()   {}
func (ExprValueInteger) i() {}
func (ExprValueString) i()  {}
func (ExprOperation) i()    {}

// ExprIdentifier holds the name of the identifier.
type ExprIdentifier struct {
	Name string
}

// ExprValueInteger holds the integer value.
type ExprValueInteger struct {
	Value string
}

// ExprValueString holds the string value.
type ExprValueString struct {
	Value string
}

// ExprOperation represents operation with == or AND operators.
type ExprOperation struct {
	Left     Expr
	Operator Operator
	Right    Expr
}

type parseFunc func(*parser) parseFunc

func (p *parser) run() {
	for state := parseStatement; state != nil; {
		state = state(p)
	}

	p.lexer.drain()
}

func (p *parser) next(skipSpace bool) token {
	for {
		t := p.lexer.nextToken()
		if !(skipSpace && t.tokenType == tokenSpace) {
			return t
		}
	}
}

func (p *parser) errorf(format string, args ...interface{}) parseFunc {
	// TODO: add token position

	return p.error(fmt.Errorf(format, args...))
}

func (p *parser) error(err error) parseFunc {
	p.err = err

	return nil
}

func (p *parser) scanFor(tokenTypes ...tokenType) (token, error) {
	t := p.next(true)
	if t.tokenType == tokenError {
		return token{}, fmt.Errorf(t.value)
	}

	expectedTokens := []string{}
	for _, tokenType := range tokenTypes {
		if tokenType == t.tokenType {
			return t, nil
		}

		expectedTokens = append(expectedTokens, tokenType.String())
	}

	return token{}, fmt.Errorf("expected %s, but got %s: \"%s\"", strings.Join(expectedTokens, ", "), t.tokenType, t.value)
}

func (p *parser) statementReady(statement Statement) parseFunc {
	p.statement = statement

	return nil
}

func parseStatement(p *parser) parseFunc {
	t := p.next(true)

	switch t.tokenType {
	case tokenError:
		return p.errorf(t.value)
	case tokenSelect:
		return parseSelect
	case tokenInsert:
		return parseInsert
	case tokenUpdate:
		return parseUpdate
	case tokenDelete:
		return parseDelete
	case tokenCreate:
		return parseCreateTable
	case tokenDrop:
		return parseDropTable
	default:
		return p.errorf(
			"expected %s, %s, %s, %s, %s or %s, but got %s: %s",
			tokenSelect,
			tokenInsert,
			tokenUpdate,
			tokenDelete,
			tokenCreate,
			tokenDrop,
			t.tokenType,
			t.value,
		)
	}
}

// parseSelect initiates SELECT statement parsing
func parseSelect(p *parser) parseFunc {
	s := &Select{}
	for {
		t, err := p.scanFor(tokenIdentifier)
		if err != nil {
			return p.error(err)
		}

		s.Columns = append(s.Columns, t.value)

		t, err = p.scanFor(tokenFrom, tokenDelimeter)
		if err != nil {
			return p.error(err)
		}

		if t.tokenType == tokenFrom {
			break
		}
	}

	t, err := p.scanFor(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	s.Table = t.value

	t, err = p.scanFor(tokenWhere, tokenLimit, tokenEnd)
	if err != nil {
		return p.error(err)
	}

	if t.tokenType == tokenWhere {
		expr, nextToken, err := parseExprOperation(p, tokenLimit, tokenEnd)
		if err != nil {
			return p.error(err)
		}

		s.Where = &Where{expr}
		t = *nextToken
	}

	if t.tokenType == tokenLimit {
		t, err = p.scanFor(tokenInteger)
		if err != nil {
			return p.error(err)
		}

		s.Limit = t.value
	}

	return p.statementReady(s)
}

// parseInsert parses INSERT statement.
func parseInsert(p *parser) parseFunc {
	t, err := p.scanFor(tokenIdentifier, tokenInto)
	if err != nil {
		return p.error(err)
	}

	if t.tokenType == tokenInto {
		// INTO is an optional keyword
		t, err = p.scanFor(tokenIdentifier)
		if err != nil {
			return p.error(err)
		}
	}

	i := &Insert{t.value, nil, nil}

	_, err = p.scanFor(tokenLeftParenthesis)
	if err != nil {
		return p.error(err)
	}

	i.Columns, err = p.parseInsertValues(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenValues)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenLeftParenthesis)
	if err != nil {
		return p.error(err)
	}

	i.Values, err = p.parseInsertValues(tokenInteger, tokenString)
	if err != nil {
		return p.error(err)
	}

	_, err = p.scanFor(tokenEnd)
	if err != nil {
		return p.error(err)
	}

	return p.statementReady(i)
}

func (p *parser) parseInsertValues(tokenTypes ...tokenType) ([]string, error) {
	values := make([]string, 0)
	for {
		t, err := p.scanFor(tokenTypes...)
		if err != nil {
			return nil, err
		}

		values = append(values, t.value)

		t, err = p.scanFor(tokenDelimeter, tokenRightParenthesis)
		if err != nil {
			return nil, err
		}

		if t.tokenType == tokenRightParenthesis {
			break
		}
	}

	return values, nil
}

// parseUpdate parses UPDATE statement.
func parseUpdate(p *parser) parseFunc {
	t, err := p.scanFor(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	u := &Update{t.value, []string{}, []string{}, nil}

	_, err = p.scanFor(tokenSet)
	if err != nil {
		return p.error(err)
	}

	for {
		t, err = p.scanFor(tokenIdentifier)
		if err != nil {
			return p.error(err)
		}

		u.Columns = append(u.Columns, t.value)

		t, err = p.scanFor(tokenAssign)
		if err != nil {
			return p.error(err)
		}

		t, err = p.scanFor(tokenString, tokenInteger)
		if err != nil {
			return p.error(err)
		}

		u.Values = append(u.Values, t.value)

		t, err = p.scanFor(tokenDelimeter, tokenWhere, tokenEnd)
		if err != nil {
			return p.error(err)
		}

		if t.tokenType == tokenEnd || t.tokenType == tokenWhere {
			break
		}
	}

	if t.tokenType == tokenWhere {
		expr, nextToken, err := parseExprOperation(p, tokenLimit, tokenEnd)
		if err != nil {
			return p.error(err)
		}

		u.Where = &Where{expr}
		t = *nextToken
	}

	return p.statementReady(u)
}

// parseDelete parses DELETE statement.
func parseDelete(p *parser) parseFunc {
	t, err := p.scanFor(tokenFrom)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	d := &Delete{t.value, nil}

	t, err = p.scanFor(tokenWhere, tokenEnd)
	if err != nil {
		return p.error(err)
	}

	if t.tokenType == tokenWhere {
		expr, nextToken, err := parseExprOperation(p, tokenLimit, tokenEnd)
		if err != nil {
			return p.error(err)
		}

		d.Where = &Where{expr}
		t = *nextToken
	}

	return p.statementReady(d)
}

// parseCreateTable parses CREATE TABLE statement.
func parseCreateTable(p *parser) parseFunc {
	t, err := p.scanFor(tokenTable)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	createTable := &CreateTable{t.value, []ColumnDefinition{}, EngineDefault}

	_, err = p.scanFor(tokenLeftParenthesis)
	if err != nil {
		return p.error(err)
	}

	for {
		t, err := p.scanFor(tokenIdentifier)
		if err != nil {
			return p.error(err)
		}

		columnName := t.value

		t, err = p.scanFor(tokenTypeInteger, tokenTypeString)
		if err != nil {
			return p.error(err)
		}

		var columnType ColumnType
		switch t.tokenType {
		case tokenTypeInteger:
			columnType = TypeInteger
		case tokenTypeString:
			columnType = TypeString
		}

		createTable.Columns = append(createTable.Columns, ColumnDefinition{columnName, columnType})

		t, err = p.scanFor(tokenDelimeter, tokenRightParenthesis)
		if err != nil {
			return p.error(err)
		}

		if t.tokenType == tokenRightParenthesis {
			break
		}
	}

	t, err = p.scanFor(tokenEngine, tokenEnd)
	if err != nil {
		return p.error(err)
	}

	if t.tokenType == tokenEnd {
		return p.statementReady(createTable)
	}

	_, err = p.scanFor(tokenAssign)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenBPTree, tokenLSM)
	if err != nil {
		return p.error(err)
	}

	switch t.tokenType {
	case tokenLSM:
		createTable.Engine = EngineLSM
	case tokenBPTree:
		createTable.Engine = EngineBPTree
	}

	_, err = p.scanFor(tokenEnd)
	if err != nil {
		return p.error(err)
	}

	return p.statementReady(createTable)
}

// parseDropTable parses DROP TABLE statement.
func parseDropTable(p *parser) parseFunc {
	t, err := p.scanFor(tokenTable)
	if err != nil {
		return p.error(err)
	}

	t, err = p.scanFor(tokenIdentifier)
	if err != nil {
		return p.error(err)
	}

	dropTable := &DropTable{t.value}

	_, err = p.scanFor(tokenEnd)
	if err != nil {
		return p.error(err)
	}

	return p.statementReady(dropTable)
}

func parseExprOperation(p *parser, terminalTokenTypes ...tokenType) (Expr, *token, error) {
	t, err := p.scanFor(tokenIdentifier, tokenInteger, tokenString)
	if err != nil {
		return nil, nil, err
	}

	var left Expr
	switch t.tokenType {
	case tokenIdentifier:
		left = ExprIdentifier{t.value}
	case tokenInteger:
		left = ExprValueInteger{t.value}
	case tokenString:
		left = ExprValueString{t.value}
	}

	t, err = p.scanFor(tokenEquals)
	if err != nil {
		return nil, nil, err
	}

	var operator Operator
	switch t.tokenType {
	case tokenEquals:
		operator = OperatorEquals
	}

	t, err = p.scanFor(tokenIdentifier, tokenInteger, tokenString)
	if err != nil {
		return nil, nil, err
	}

	var right Expr
	switch t.tokenType {
	case tokenIdentifier:
		right = ExprIdentifier{t.value}
	case tokenInteger:
		right = ExprValueInteger{t.value}
	case tokenString:
		right = ExprValueString{t.value}
	}

	var expr = ExprOperation{left, operator, right}

	expected := make([]tokenType, len(terminalTokenTypes))
	copy(expected, terminalTokenTypes)
	expected = append(expected, tokenAnd)

	t, err = p.scanFor(expected...)
	if err != nil {
		return nil, nil, err
	}

	switch t.tokenType {
	case tokenAnd:
		right, t, err := parseExprOperation(p, terminalTokenTypes...)
		if err != nil {
			return nil, nil, err
		}

		return ExprOperation{expr, OperatorLogicalAnd, right}, t, nil
	}

	return expr, &t, nil
}
