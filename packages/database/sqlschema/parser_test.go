package sqlschema

// import (
// 	"fmt"
// 	"reflect"
// 	"testing"

// 	"encoding/json"

// 	sql "github.com/krasun/gosqlparser"
// )

// func Example() {
// 	query, err := sql.Parse("SELECT col1, col2 FROM table1 WHERE col1 == \"abc\" AND col3 == 5 LIMIT 10")
// 	if err != nil {
// 		fmt.Printf("unexpected error: %s", err)
// 		return
// 	}

// 	json, err := json.Marshal(query)
// 	if err != nil {
// 		fmt.Printf("unexpected error: %s", err)
// 		return
// 	}

// 	fmt.Println(string(json))
// 	// Output:
// 	// {"Table":"table1","Columns":["col1","col2"],"Where":{"Expr":{"Left":{"Left":{"Name":"col1"},"Operator":0,"Right":{"Value":"\"abc\""}},"Operator":1,"Right":{"Left":{"Name":"col3"},"Operator":0,"Right":{"Value":"5"}}}},"Limit":"10"}
// }

// func TestParser(t *testing.T) {
// 	testCases := []struct {
// 		name              string
// 		input             string
// 		expectedStatement sql.Statement
// 		err               error
// 	}{
// 		{
// 			"broken statement",
// 			"table1",
// 			nil,
// 			fmt.Errorf("expected SELECT, INSERT, UPDATE, DELETE, CREATE or DROP, but got identifier: table1"),
// 		},
// 		{
// 			"broken statement",
// 			"`",
// 			nil,
// 			fmt.Errorf("unexpected rune"),
// 		},
// 		{
// 			"unfinished SELECT statement",
// 			"SELECT table1",
// 			nil,
// 			fmt.Errorf("expected FROM, delimeter, but got end: \"\""),
// 		},
// 		{
// 			"unfinished SELECT FROM statement",
// 			"SELECT col1 FROM",
// 			nil,
// 			fmt.Errorf("expected identifier, but got end: \"\""),
// 		},
// 		{
// 			"unfinished SELECT FROM WHERE statement",
// 			"SELECT col FROM table1 WHERE",
// 			nil,
// 			fmt.Errorf("expected identifier, integer, string, but got end: \"\""),
// 		},
// 		{
// 			"unfinished WHERE statement",
// 			"SELECT col FROM table1 WHERE a ==",
// 			nil,
// 			fmt.Errorf("expected identifier, integer, string, but got end: \"\""),
// 		},
// 		{
// 			"unfinished WHERE statement",
// 			"SELECT col FROM table1 WHERE a",
// 			nil,
// 			fmt.Errorf("expected equals, but got end: \"\""),
// 		},
// 		{
// 			"unfinished WHERE statement",
// 			"SELECT col FROM table1 LIMIT",
// 			nil,
// 			fmt.Errorf("expected integer, but got end: \"\""),
// 		},
// 		{
// 			"broken SELECT statement",
// 			"SELECT col, FROM table1 LIMIT",
// 			nil,
// 			fmt.Errorf("expected identifier, but got FROM: \"FROM\""),
// 		},
// 		{
// 			"unfinished DELETE FROM statement",
// 			"DELETE FROM",
// 			nil,
// 			fmt.Errorf("expected identifier, but got end: \"\""),
// 		},
// 		{
// 			"full CREATE TABLE query",
// 			"CREATE TABLE table1 (col1 INTEGER, col2 STRING)",
// 			&sql.CreateTable{"table1", []sql.ColumnDefinition{{"col1", sql.TypeInteger}, {"col2", sql.TypeString}}, sql.EngineDefault},
// 			nil,
// 		},
// 		{
// 			"full CREATE TABLE query with LSM engine",
// 			"CREATE TABLE table1 (col1 INTEGER, col2 STRING) ENGINE=LSM",
// 			&sql.CreateTable{"table1", []sql.ColumnDefinition{{"col1", sql.TypeInteger}, {"col2", sql.TypeString}}, sql.EngineLSM},
// 			nil,
// 		},
// 		{
// 			"full CREATE TABLE query with B+ tree engine",
// 			"CREATE TABLE table1 (col1 INTEGER, col2 STRING) ENGINE=BPTREE",
// 			&sql.CreateTable{"table1", []sql.ColumnDefinition{{"col1", sql.TypeInteger}, {"col2", sql.TypeString}}, sql.EngineBPTree},
// 			nil,
// 		},
// 		{
// 			"full DROP TABLE query",
// 			"DROP TABLE table1",
// 			&sql.DropTable{"table1"},
// 			nil,
// 		},
// 		{
// 			"broken DROP TABLE",
// 			"DROP table1",
// 			nil,
// 			fmt.Errorf("expected TABLE, but got identifier: \"table1\""),
// 		},
// 		{
// 			"simple SELECT FROM",
// 			"SELECT col1, col2 FROM table1",
// 			&sql.Select{"table1", []string{"col1", "col2"}, nil, ""},
// 			nil,
// 		},
// 		{
// 			"simple DELETE FROM",
// 			"DELETE FROM table1",
// 			&sql.Delete{"table1", nil},
// 			nil,
// 		},
// 		{
// 			"DELETE FROM WHERE",
// 			"DELETE FROM table1 WHERE col1 == col2",
// 			&sql.Delete{"table1", &sql.Where{sql.ExprOperation{sql.ExprIdentifier{"col1"}, sql.OperatorEquals, sql.ExprIdentifier{"col2"}}}},
// 			nil,
// 		},
// 		{
// 			"simple INSERT INTO",
// 			"INSERT INTO table1 (col1, col2) VALUES (\"val1\", 25)",
// 			&sql.Insert{"table1", []string{"col1", "col2"}, []string{"\"val1\"", "25"}},
// 			nil,
// 		},
// 		{
// 			"simple UPDATE",
// 			"UPDATE table1 SET col1 = \"val1\", col2 = 2",
// 			&sql.Update{"table1", []string{"col1", "col2"}, []string{"\"val1\"", "2"}, nil},
// 			nil,
// 		},
// 		{
// 			"simple UPDATE",
// 			"UPDATE table1 SET col1 = \"val1\", col2 = 2 WHERE col1 == col2",
// 			&sql.Update{"table1", []string{"col1", "col2"}, []string{"\"val1\"", "2"}, &sql.Where{sql.ExprOperation{sql.ExprIdentifier{"col1"}, sql.OperatorEquals, sql.ExprIdentifier{"col2"}}}},
// 			nil,
// 		},
// 		{
// 			"SELECT FROM with LIMIT",
// 			"SELECT col1, col2 FROM table1 LIMIT 10",
// 			&sql.Select{"table1", []string{"col1", "col2"}, nil, "10"},
// 			nil,
// 		},
// 		{
// 			"SELECT FROM with simple WHERE",
// 			"SELECT col1, col2 FROM table1 WHERE col1 == col2",
// 			&sql.Select{"table1", []string{"col1", "col2"}, &sql.Where{sql.ExprOperation{sql.ExprIdentifier{"col1"}, sql.OperatorEquals, sql.ExprIdentifier{"col2"}}}, ""},
// 			nil,
// 		},
// 		{
// 			"SELECT FROM with WHERE AND LIMIT",
// 			"SELECT col1, col2 FROM table1 WHERE col1 == col2 AND col3 == col4 LIMIT 10",
// 			&sql.Select{
// 				"table1",
// 				[]string{"col1", "col2"},
// 				&sql.Where{
// 					sql.ExprOperation{
// 						sql.ExprOperation{
// 							sql.ExprIdentifier{"col1"}, sql.OperatorEquals, sql.ExprIdentifier{"col2"},
// 						},
// 						sql.OperatorLogicalAnd,
// 						sql.ExprOperation{
// 							sql.ExprIdentifier{"col3"}, sql.OperatorEquals, sql.ExprIdentifier{"col4"},
// 						},
// 					},
// 				},
// 				"10",
// 			},
// 			nil,
// 		},
// 		{
// 			"SELECT FROM with WHERE AND VALUES",
// 			"SELECT col1, col2 FROM table1 WHERE col1 == \"abc\" AND col3 == 5 LIMIT 10",
// 			&sql.Select{
// 				"table1",
// 				[]string{"col1", "col2"},
// 				&sql.Where{
// 					sql.ExprOperation{
// 						sql.ExprOperation{
// 							sql.ExprIdentifier{"col1"}, sql.OperatorEquals, sql.ExprValueString{"\"abc\""},
// 						},
// 						sql.OperatorLogicalAnd,
// 						sql.ExprOperation{
// 							sql.ExprIdentifier{"col3"}, sql.OperatorEquals, sql.ExprValueInteger{"5"},
// 						},
// 					},
// 				},
// 				"10",
// 			},
// 			nil,
// 		},
// 	}

// 	for _, testCase := range testCases {
// 		t.Run(testCase.name, func(t *testing.T) {
// 			statement, err := sql.Parse(testCase.input)
// 			if testCase.err != nil {
// 				if err == nil {
// 					t.Errorf("expected error \"%s\", but got nil", testCase.err)
// 				} else if testCase.err.Error() != err.Error() {
// 					t.Errorf("expected error \"%s\", but got \"%s\"", testCase.err, err)
// 				}
// 			} else {
// 				if err != nil {
// 					t.Errorf("unexpected error \"%s\"", err)
// 				}
// 			}

// 			if testCase.expectedStatement != nil && !reflect.DeepEqual(testCase.expectedStatement, statement) {
// 				t.Errorf("expected %v, but got %v", testCase.expectedStatement, statement)
// 			}
// 		})
// 	}
// }
