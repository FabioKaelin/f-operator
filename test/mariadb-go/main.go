package main

// mariadb connection

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// struct with methods

type MariaDB struct {
	db *sql.DB
}

type asdf struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

func Init() MariaDB {
	// get env vars
	dbHost := "mariadb-internal-service.test.svc.cluster.local"
	// dbHost := "mariadb-internal-service.ivp-foperator.svc.cluster.local"
	dbPort := "3306"
	// dbPort := "8080"
	dbUser := "root"
	// dbPass := "mysecretpassword"
	dbPass := "mypassword"
	dbName := "testdb"

	// connection string
	db, err := sql.Open("mysql", dbUser+":"+dbPass+"@tcp("+dbHost+":"+dbPort+")/"+dbName)
	if err != nil {
		panic(err.Error())
	}

	// set max open connections
	// db.SetMaxOpenConns(100)

	// set max idle connections
	// db.SetMaxIdleConns(100)

	// set max connection lifetime
	// db.SetConnMaxLifetime(time.Minute * 5)

	return MariaDB{db: db}
}

func (m *MariaDB) Query(query string) *sql.Rows {
	// execute query
	rows, err := m.db.Query(query)
	if err != nil {
		panic(err.Error())
	}

	return rows
}

func main() {

	// init log

	// create simple api
	// create a http endpoint

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, "hello-world")
	})
	router.GET("/call", func(c *gin.Context) {
		// init mariadb
		mariadb := Init()
		// query
		rows := mariadb.Query("SELECT * FROM asdf")

		asdfs := []asdf{}

		// loop rows
		for rows.Next() {
			var id int
			var name string
			err := rows.Scan(&id, &name)
			if err != nil {
				panic(err.Error())
			}

			// log
			asdfs = append(asdfs, asdf{Id: id, Name: name})
			fmt.Println(id, name)
			fmt.Println("aaaa")
			spew.Dump(asdfs)
			fmt.Println("bbbb")
			spew.Dump(asdf{Id: id, Name: name})
			fmt.Println("....")
		}
		fmt.Println("--------------")
		spew.Dump(asdfs)
		c.IndentedJSON(http.StatusOK, asdfs)
	})

	router.Run("0.0.0.0:8080")

}
