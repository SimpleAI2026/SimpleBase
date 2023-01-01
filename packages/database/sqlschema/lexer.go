package sqlschema

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// lexer tokenizes the input and produces tokens.
type lexer struct {
	input string // raw SQL query

	start    int // start position of the current token
	position int // current position
	width    int // width of last rune read from input

	tokens chan token
}

// token is an entity produced by tokenizer for parser that represents a smaller typed piece
// of input string.
type token struct {
	tokenType tokenType
	value     string
}

type tokenType int

const (
	tokenError            tokenType = iota
	tokenSpace                      // whitespace
	tokenIdentifier                 // table or column name
	tokenEnd                        // the end of the input
	tokenEquals                     // "=="
	tokenAssign                     // "="
	tokenDelimeter                  // ','
	tokenLeftParenthesis            // '('
	tokenRightParenthesis           // ')'
	tokenInteger                    // integer
	tokenString                     // string including quotes
	tokenAnd                        // AND
	tokenInsert                     // INSERT
	tokenInto                       // INTO
	tokenSelect                     // SELECT
	tokenDelete                     // DELETE
	tokenFrom                       // FROM
	tokenWhere                      // WHERE
	tokenLimit                      // LIMIT
	tokenValues                     // VALUES
	tokenUpdate                     // UPDATE
	tokenSet                        // SET
	tokenCreate                     // CREATE
	tokenDrop                       // DROP
	tokenTable                      // TABLE
	tokenTypeInteger                // INTEGER
	tokenTypeString                 // STRING
	tokenEngine                     // ENGINE
	tokenLSM                        // LSM
	tokenBPTree                     // B+ tree
)

var tokenToString = map[tokenType]string{
	tokenError:            "error",
	tokenSpace:            "space",
	tokenIdentifier:       "identifier",
	tokenEnd:              "end",
	tokenEquals:           "equals",
	tokenAssign:           "assign",
	tokenDelimeter:        "delimeter",
	tokenLeftParenthesis:  "leftParenthesis",
	tokenRightParenthesis: "rightParenthesis",
	tokenInteger:          "integer",
	tokenString:           "string",
	tokenAnd:              "AND",
	tokenInsert:           "INSERT",
	tokenInto:             "INTO",
	tokenSelect:           "SELECT",
	tokenDelete:           "DELETE",
	tokenFrom:             "FROM",
	tokenWhere:            "WHERE",
	tokenLimit:            "LIMIT",
	tokenValues:           "VALUES",
	tokenUpdate:           "UPDATE",
	tokenSet:              "SET",
	tokenCreate:           "CREATE",
	tokenDrop:             "DROP",
	tokenTable:            "TABLE",
	tokenTypeInteger:      "typeInteger",
	tokenEngine:           "ENGINE",
	tokenLSM:              "LSM",
	tokenBPTree:           "BPTree",
}

func (t tokenType) String() string {
	s, defined := tokenToString[t]
	if defined {
		return s
	}

	return "unknown"
}

const (
	keywordSelect  = "SELECT"
	keywordInsert  = "INSERT"
	keywordDelete  = "DELETE"
	keywordInto    = "INTO"
	keywordFrom    = "FROM"
	keywordWhere   = "WHERE"
	keywordLimit   = "LIMIT"
	keywordAnd     = "AND"
	keywordValues  = "VALUES"
	keywordUpdate  = "UPDATE"
	keywordSet     = "SET"
	keywordInteger = "INTEGER"
	keywordString  = "STRING"
	keywordCreate  = "CREATE"
	keywordDrop    = "DROP"
	keywordTable   = "TABLE"
	keywordEngine  = "ENGINE"
	keywordLSM     = "LSM"
	keywordBPTree  = "BPTREE"
)

var keywords = map[string]tokenType{
	keywordSelect:  tokenSelect,
	keywordFrom:    tokenFrom,
	keywordWhere:   tokenWhere,
	keywordLimit:   tokenLimit,
	keywordAnd:     tokenAnd,
	keywordInsert:  tokenInsert,
	keywordInto:    tokenInto,
	keywordValues:  tokenValues,
	keywordDelete:  tokenDelete,
	keywordUpdate:  tokenUpdate,
	keywordSet:     tokenSet,
	keywordString:  tokenTypeString,
	keywordInteger: tokenTypeInteger,
	keywordCreate:  tokenCreate,
	keywordTable:   tokenTable,
	keywordDrop:    tokenDrop,
	keywordEngine:  tokenEngine,
	keywordLSM:     tokenLSM,
	keywordBPTree:  tokenBPTree,
}

const end = -1

type lexFunc func(*lexer) lexFunc

// newLexer returns an instance of the new lexer.
func newLexer(input string) *lexer {
	l := &lexer{
		input:  input,
		tokens: make(chan token),
	}

	return l
}

func (l *lexer) run() {
	for state := lexStatement; state != nil; {
		state = state(l)
	}

	close(l.tokens)
}

// nextToken consumes token and returns the next token.
func (l *lexer) nextToken() token {
	return <-l.tokens
}

// produce sends the token.
func (l *lexer) produce(t tokenType) {
	l.tokens <- token{t, l.input[l.start:l.position]}
	l.start = l.position
}

func (l *lexer) next() rune {
	if l.position >= len(l.input) {
		l.width = 0

		return end
	}

	r, w := utf8.DecodeRuneInString(l.input[l.position:])

	l.width = w
	l.position += w

	return r
}

func (l *lexer) revert() {
	l.position -= l.width
}

func (l *lexer) peek() rune {
	r := l.next()
	l.revert()

	return r
}

func (l *lexer) drain() {
	for range l.tokens {
	}
}

func lexStatement(l *lexer) lexFunc {
	r := l.next()

	switch true {
	case unicode.IsDigit(r):
		return lexInteger
	case isAlphaNumeric(r):
		return lexIdentifier
	case unicode.IsSpace(r):
		l.produce(tokenSpace)

		return lexStatement
	case r == '(':
		l.produce(tokenLeftParenthesis)
		return lexStatement
	case r == ')':
		l.produce(tokenRightParenthesis)
		return lexStatement
	case r == '"':
		return lexString
	case r == ',':
		l.produce(tokenDelimeter)
		return lexStatement
	case r == '=':
		switch l.peek() {
		case '=':
			l.next()
			l.produce(tokenEquals)
		default:
			l.produce(tokenAssign)
		}

		return lexStatement
	case r == end:

		l.produce(tokenEnd)
		return nil
	}

	return l.errorf("unexpected rune")
}

// lexInteger lexes an integer.
func lexInteger(l *lexer) lexFunc {
	r := l.peek()
	if unicode.IsDigit(r) {
		l.next()

		return lexInteger
	}

	l.produce(tokenInteger)

	return lexStatement
}

// lexString lexes a quoted string.
func lexString(l *lexer) lexFunc {
	r := l.next()

	switch r {
	case '"':
		l.produce(tokenString)

		return lexStatement
	case end:
		return l.errorf("expected \"")
	}

	return lexString
}

func lexIdentifier(l *lexer) lexFunc {
	r := l.next()

	if isAlphaNumeric(r) {
		// advance
		return lexIdentifier
	}

	l.revert()

	word := l.input[l.start:l.position]
	if t, ok := keywords[strings.ToUpper(word)]; ok {
		l.produce(t)
	} else {
		l.produce(tokenIdentifier)
	}

	return lexStatement
}

func (l *lexer) errorf(format string, args ...interface{}) lexFunc {
	l.tokens <- token{tokenError, fmt.Sprintf(format, args...)}

	return nil
}

func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
