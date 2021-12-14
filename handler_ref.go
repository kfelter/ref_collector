package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	yugeGIF       = "https://media1.giphy.com/media/j3IxJRLNLZz9sXR7ZA/giphy.gif?cid=ecf05e47diq003c3175znofrtmafu403shyfpswksd8wtd4y&rid=giphy.gif&ct=g"
	helloThereGIF = "https://c.tenor.com/qA9u4ETE66MAAAAC/hello-there-kenobi.gif"
	whatIsThisGIF = "https://media4.giphy.com/media/3ohuAAAIvICvEs4Psc/giphy.gif?cid=ecf05e47yoxb3as45q2uc26c2ehzb73n3cjfbbid5vko5l4x&rid=giphy.gif&ct=g"
	haltGIF       = "https://media0.giphy.com/media/tB8Wl0JABkSkQa7vGE/giphy.gif?cid=ecf05e47e3evasivauciept7kb15gipxut9vnctmhnk787ay&rid=giphy.gif&ct=g"
)

var (
	blocked = os.Getenv("BLOCKED_IPS")
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

func refHandler(rw http.ResponseWriter, r *http.Request) {
	// get query vars
	refName := r.URL.Query().Get("ref")
	if refName == "" {
		refName = "unknown"
	}
	if len(refName) > 40 {
		http.Redirect(rw, r, yugeGIF, http.StatusBadRequest)
		log.Println("ref name too large", r.URL.String(), r.RemoteAddr)
		return
	}
	dst := r.URL.Query().Get("dst")
	if dst == "" {
		dst = defaultDest
	}
	if _, err := url.Parse(dst); err != nil {
		http.Redirect(rw, r, whatIsThisGIF, http.StatusBadRequest)
		log.Println("dst is not valid", r.URL.String(), r.RemoteAddr)
		return
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

	// check if the ip address is blocked
	if strings.Contains(blocked, addr) {
		dst = helloThereGIF
		log.Println("blocked ip attempted request", r.URL.String())
		return
	}

	// check if the ip address is making too many requests
	now := time.Now()
	t0 := now.Add(-5 * time.Minute)
	count, err := countRequests(addr, t0.UnixNano(), now.UnixNano())
	log.Println("ip", addr, "made", count, "requests in", now.Sub(t0).String(), "err", err)
	if count > 10 {
		http.Redirect(rw, r, haltGIF, http.StatusBadRequest)
		log.Println("too many requests", r.URL.String(), r.RemoteAddr)
		return
	}

	var (
		loc *locInfo
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
