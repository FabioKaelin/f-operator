package main

// mariadb connection

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// struct with methods

type MariaDB struct {
	db *sql.DB
}

type asdf struct {
	id   int
	name string
}

func Init() MariaDB {
	// get env vars
	dbHost := "mariadb-internal-service.ivp-foperator.svc.cluster.local"
	dbPort := "8080"
	dbUser := "root"
	dbPass := "mypassword"
	dbName := "testdb"

	// connection string
	db, err := sql.Open("mysql", dbUser+":"+dbPass+"@tcp("+dbHost+":"+dbPort+")/"+dbName)
	if err != nil {
		panic(err.Error())
	}

	// set max open connections
	db.SetMaxOpenConns(100)

	// set max idle connections
	db.SetMaxIdleConns(100)

	// set max connection lifetime
	db.SetConnMaxLifetime(time.Minute * 5)

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
			asdfs = append(asdfs, asdf{id: id, name: name})
			fmt.Println(id, name)
		}
		c.IndentedJSON(http.StatusOK, asdfs)
	})

	router.Run("0.0.0.0:8080")

}
