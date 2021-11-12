package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
)

func newPinsHandler(rw http.ResponseWriter, r *http.Request) {
	type res struct {
		PinHash string `json:"pin_hash"`
		Pin     string `json:"pin"`
		Info    string `json:"info"`
	}

	response := new(res)

	response.Pin = r.URL.Query().Get("pin")
	if response.Pin == "" {
		response.Pin = fmt.Sprintf("%d", rand.Intn(8999)+1000)
	}

	response.PinHash = fmt.Sprintf("%x", hasher.Sum([]byte(response.Pin))[:3])

	response.Info = "https://ref-collector-2021.herokuapp.com/info"

	data, _ := json.MarshalIndent(response, "", "  ")
	rw.WriteHeader(200)
	rw.Write(data)
}
