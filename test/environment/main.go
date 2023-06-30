package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	fmt.Println("FOO", os.Getenv("FOO"))
	fmt.Println("BAR", os.Getenv("BAR"))
	fmt.Println("BAZ", os.Getenv("BAZ"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		variables := ""
		variables += fmt.Sprintf("FOO: %s<br>", os.Getenv("FOO"))
		variables += fmt.Sprintf("BAR: %s<br>", os.Getenv("BAR"))
		variables += fmt.Sprintf("BAZ: %s<br>", os.Getenv("BAZ"))
		fmt.Fprintf(w, "<h1>Hello World</h1><br>"+variables)
	})
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<h1>Hello World This is the API</h1>")
	})

	http.ListenAndServe(":8080", nil)
}
