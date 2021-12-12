package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
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
	Continent   string  `json:"continent,omitempty"`
	Country     string  `json:"country,omitempty"`
	Region      string  `json:"region,omitempty"`
	City        string  `json:"city,omitempty"`
	Zip         string  `json:"zip,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	TimeHuman   string  `json:"time_human,omitempty"`
}

func (e Event) String() string {
	data, _ := json.Marshal(e)
	return string(data)
}

var (
	blocked = os.Getenv("BLOCKED_IPS")
)

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

	if strings.Contains(blocked, addr) {
		refName = "BLOCKED+" + refName
		dst = "https://c.tenor.com/qA9u4ETE66MAAAAC/hello-there-kenobi.gif"
		log.Println("blocked ip attempted request", r.URL.String())
	}

	var (
		loc *locInfo
		err error
	)
	userAgent := r.Header.Get("User-Agent")
	if !strings.Contains(userAgent, "bot") &&
		!strings.Contains(userAgent, "ahrefs") {
		ctx, cancel := context.WithTimeout(context.Background(), locTimeout)
		defer cancel()

		loc, err = getLoc(ctx, addr)
		if err != nil {
			log.Println("error getting location data", err)
		}
	} else {
		loc = &locInfo{}
	}

	createdAt := time.Now().UnixNano()
	_, err = db.Exec(r.Context(), `insert into ref(id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude, pin_hash) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		id,
		createdAt,
		refName,
		dst,
		addr,
		userAgent,
		loc.Continent,
		loc.Country,
		loc.Region,
		loc.City,
		loc.Zip,
		loc.Latitude,
		loc.Longitude,
		pinHash,
	)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	log.Println("event", Event{ID: id, CreatedAt: createdAt, Name: refName, Dest: dst, RequestAddr: addr, UserAgent: userAgent}, "pinhash", pinHash)
	http.Redirect(rw, r, dst, http.StatusTemporaryRedirect)
	return
}
