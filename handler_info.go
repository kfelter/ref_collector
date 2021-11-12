package main

import "net/http"

func infoHandler(rw http.ResponseWriter, r *http.Request) {
	data, err := embedFS.ReadFile("embed/info.html")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	rw.WriteHeader(200)
	rw.Header().Add("Content-Type", "text/html")
	rw.Write(data)
}
