package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server failed to start: %v\n", err)
	}
}
