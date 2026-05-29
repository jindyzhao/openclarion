package main

import "net/http"

func run() *http.Client {
	return http.DefaultClient
}
