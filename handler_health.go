package main

import "net/http"

func healthHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(`{"status":"OK"}`))
	return
}
