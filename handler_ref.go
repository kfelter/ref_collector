package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID          string  `json:"id"`
	CreatedAt   int64   `json:"created_at"`
	Name        string  `json:"name"`
	Dest        string  `json:"dst"`
	RequestAddr string  `json:"request_addr"`
	UserAgent   string  `json:"user_agent"`
	Continent   string  `json:"continent"`
	Country     string  `json:"country"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Zip         string  `json:"zip"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	TimeHuman   string  `json:"time_human"`
}

func (e Event) String() string {
	data, _ := json.Marshal(e)
	return string(data)
}

func refHandler(rw http.ResponseWriter, r *http.Request) {
	// get query vars
	refName := r.URL.Query().Get("ref")
	if refName == "" {
		refName = "unknown"
	}
	dst := r.URL.Query().Get("dst")
	if dst == "" {
		dst = defaultDest
	}
	pinHash := r.URL.Query().Get("pin_hash")
	if pinHash == "" {
		pinHash = defaultPinHash
	}

	id := r.Header.Get("X-Request-Id")
	if id == "" {
		id = uuid.New().String()
	}
	addr := r.Header.Get("X-Forwarded-For")
	if ss := strings.Split(addr, ","); len(ss) > 1 {
		addr = ss[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), locTimeout)
	defer cancel()

	locInfo, err := getLoc(ctx, addr)
	if err != nil {
		log.Println("error getting location data", err)
		// TODO: remove 500 error after confirming it works
		http.Error(rw, err.Error(), 500)
		return
	}

	_, err = db.Exec(r.Context(), `insert into ref(id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude, pin_hash) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		id,
		time.Now().UnixNano(),
		refName,
		dst,
		addr,
		r.Header.Get("User-Agent"),
		locInfo.Continent,
		locInfo.Country,
		locInfo.Region,
		locInfo.City,
		locInfo.Zip,
		locInfo.Latitude,
		locInfo.Longitude,
		pinHash,
	)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	http.Redirect(rw, r, dst, http.StatusTemporaryRedirect)
	return
}
