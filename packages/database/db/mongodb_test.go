package database

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/upper/db/v4/adapter/mongo"
)

var settings = mongo.ConnectionURL{
	Database: `upperio_tests`, // Database name
	Host:     `127.0.0.1`,     // Server IP or name
}

type Birthday struct {
	// The 'name' column of the 'birthday' table
	// is mapped to the 'name' property.
	Name string `bson:"name"`
	// The 'born' column of the 'birthday' table
	// is mapped to the 'born' property.
	Born time.Time `bson:"born"`
}

func TestMongodbV1(t *testing.T) {

	// The database connection is attempted.
	sess, err := mongo.Open(settings)
	if err != nil {
		log.Fatalf("db.Open(): %q\n", err)
	}
	defer sess.Close() // Closing the session is a good practice.

	// The 'birthday' table is referenced.
	birthdayCollection := sess.Collection("birthday")

	// Any rows that might have been added between the creation of
	// the table and the execution of this function are removed.
	err = birthdayCollection.Truncate()
	if err != nil {
		log.Fatalf("Truncate(): %q\n", err)
	}

	// Three rows are inserted into the 'Birthday' table.
	birthdayCollection.Insert(Birthday{
		Name: "Hayao Miyazaki",
		Born: time.Date(1941, time.January, 5, 0, 0, 0, 0, time.Local),
	})

	birthdayCollection.Insert(Birthday{
		Name: "Nobuo Uematsu",
		Born: time.Date(1959, time.March, 21, 0, 0, 0, 0, time.Local),
	})

	birthdayCollection.Insert(Birthday{
		Name: "Hironobu Sakaguchi",
		Born: time.Date(1962, time.November, 25, 0, 0, 0, 0, time.Local),
	})

	// The database is queried for the rows inserted.
	res := birthdayCollection.Find()

	// The 'birthdays' variable is filled with the results found.
	var birthday []Birthday

	err = res.All(&birthday)
	if err != nil {
		log.Fatalf("res.All(): %q\n", err)
	}

	// The 'birthdays' variable is printed to stdout.
	for _, birthday := range birthday {
		fmt.Printf(
			"%s was born in %s.\n",
			birthday.Name,
			birthday.Born.Format("January 2, 2006"),
		)
	}
}
