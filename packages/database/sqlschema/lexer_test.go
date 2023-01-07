package sqlschema

import (
	"reflect"
	"testing"
)

func TestLexer(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedTokens []token
	}{
		{
			"select keyword only",
			"SELECT",
			[]token{
				{tokenSelect, "SELECT"},
				{tokenEnd, ""},
			},
		},
		{
			"keywords and spaces",
			"SELECT FROM WHERE LIMIT",
			[]token{
				{tokenSelect, "SELECT"},
				{tokenSpace, " "},
				{tokenFrom, "FROM"},
				{tokenSpace, " "},
				{tokenWhere, "WHERE"},
				{tokenSpace, " "},
				{tokenLimit, "LIMIT"},
				{tokenEnd, ""},
			},
		},
		{
			"keywords and identifiers",
			"SELECT field FROM table1 WHERE LIMIT",
			[]token{
				{tokenSelect, "SELECT"},
				{tokenSpace, " "},
				{tokenIdentifier, "field"},
				{tokenSpace, " "},
				{tokenFrom, "FROM"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenWhere, "WHERE"},
				{tokenSpace, " "},
				{tokenLimit, "LIMIT"},
				{tokenEnd, ""},
			},
		},
		{
			"equals only",
			"==",
			[]token{
				{tokenEquals, "=="},
				{tokenEnd, ""},
			},
		},
		{
			"keyword and equals",
			"SELECT ==",
			[]token{
				{tokenSelect, "SELECT"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenEnd, ""},
			},
		},
		{
			"full SELECT query",
			"SELECT c1, c2 FROM table1 WHERE c3 == c4 AND c5 == c6",
			[]token{
				{tokenSelect, "SELECT"},
				{tokenSpace, " "},
				{tokenIdentifier, "c1"},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenIdentifier, "c2"},
				{tokenSpace, " "},
				{tokenFrom, "FROM"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenWhere, "WHERE"},
				{tokenSpace, " "},
				{tokenIdentifier, "c3"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenIdentifier, "c4"},
				{tokenSpace, " "},
				{tokenAnd, "AND"},
				{tokenSpace, " "},
				{tokenIdentifier, "c5"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenIdentifier, "c6"},
				{tokenEnd, ""},
			},
		},
		{
			"integer",
			"123456789",
			[]token{
				{tokenInteger, "123456789"},
				{tokenEnd, ""},
			},
		},
		{
			"string",
			"\"raw string\"",
			[]token{
				{tokenString, "\"raw string\""},
				{tokenEnd, ""},
			},
		},
		{
			"full INSERT query",
			"INSERT INTO table1 (c1, c2, c3) VALUES (5, \"some string\", 10)",
			[]token{
				{tokenInsert, "INSERT"},
				{tokenSpace, " "},
				{tokenInto, "INTO"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenLeftParenthesis, "("},
				{tokenIdentifier, "c1"},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenIdentifier, "c2"},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenIdentifier, "c3"},
				{tokenRightParenthesis, ")"},
				{tokenSpace, " "},
				{tokenValues, "VALUES"},
				{tokenSpace, " "},
				{tokenLeftParenthesis, "("},
				{tokenInteger, "5"},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenString, "\"some string\""},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenInteger, "10"},
				{tokenRightParenthesis, ")"},
				{tokenEnd, ""},
			},
		},
		{
			"full DELETE query",
			"DELETE FROM table1 WHERE c1 == 5 AND c3 == \"quoted string\"",
			[]token{
				{tokenDelete, "DELETE"},
				{tokenSpace, " "},
				{tokenFrom, "FROM"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenWhere, "WHERE"},
				{tokenSpace, " "},
				{tokenIdentifier, "c1"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenInteger, "5"},
				{tokenSpace, " "},
				{tokenAnd, "AND"},
				{tokenSpace, " "},
				{tokenIdentifier, "c3"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenString, "\"quoted string\""},
				{tokenEnd, ""},
			},
		},
		{
			"full UPDATE query",
			"UPDATE table1 SET c1 = 10 WHERE c1 == 5 AND c3 == \"quoted string\"",
			[]token{
				{tokenUpdate, "UPDATE"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenSet, "SET"},
				{tokenSpace, " "},
				{tokenIdentifier, "c1"},
				{tokenSpace, " "},
				{tokenAssign, "="},
				{tokenSpace, " "},
				{tokenInteger, "10"},
				{tokenSpace, " "},
				{tokenWhere, "WHERE"},
				{tokenSpace, " "},
				{tokenIdentifier, "c1"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenInteger, "5"},
				{tokenSpace, " "},
				{tokenAnd, "AND"},
				{tokenSpace, " "},
				{tokenIdentifier, "c3"},
				{tokenSpace, " "},
				{tokenEquals, "=="},
				{tokenSpace, " "},
				{tokenString, "\"quoted string\""},
				{tokenEnd, ""},
			},
		},
		{
			"full CREATE TABLE query",
			"CREATE TABLE table1 (c1 INTEGER, c2 STRING)",
			[]token{
				{tokenCreate, "CREATE"},
				{tokenSpace, " "},
				{tokenTable, "TABLE"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenSpace, " "},
				{tokenLeftParenthesis, "("},
				{tokenIdentifier, "c1"},
				{tokenSpace, " "},
				{tokenTypeInteger, "INTEGER"},
				{tokenDelimeter, ","},
				{tokenSpace, " "},
				{tokenIdentifier, "c2"},
				{tokenSpace, " "},
				{tokenTypeString, "STRING"},
				{tokenRightParenthesis, ")"},
				{tokenEnd, ""},
			},
		},
		{
			"full DROP TABLE query",
			"DROP TABLE table1",
			[]token{
				{tokenDrop, "DROP"},
				{tokenSpace, " "},
				{tokenTable, "TABLE"},
				{tokenSpace, " "},
				{tokenIdentifier, "table1"},
				{tokenEnd, ""},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualTokens := tokenize(testCase.input)

			if !reflect.DeepEqual(testCase.expectedTokens, actualTokens) {
				t.Errorf("expected %v, but got %v", testCase.expectedTokens, actualTokens)
			}
		})
	}
}

func tokenize(input string) []token {
	l := newLexer(input)
	go l.run()

	all := make([]token, 0)
	for {
		t, ok := <-l.tokens
		if !ok {
			break
		}

		all = append(all, t)
	}

	return all
}
