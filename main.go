package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	//go:embed structure.sql
	structureSQL string
	//go:embed favicon.ico
	faviconFile    []byte
	defaultDest    = os.Getenv("DEFAULT_DEST")
	authPin        = os.Getenv("PIN")
	neutrinoAPIKey = os.Getenv("NEUTRINO_API_KEY")
	ipstackAPIKey  = os.Getenv("IPSTACK_API_KEY")
	db             *pgxpool.Pool
	locTimeout     time.Duration
)

type ref struct {
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
	Latitude    float32 `json:"latitude"`
	Longitiude  float32 `json:"longitude"`
}

func main() {
	if defaultDest == "" {
		log.Fatalln("env var DEFAULT_DEST is required")
	}
	var err error
	locTimeout, err = time.ParseDuration(os.Getenv("LOC_TIMEOUT"))
	if err != nil {
		log.Println("setting loc timeout to default 300ms")
		locTimeout = 300 * time.Millisecond
	}

	poolConfig, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalln("Unable to parse DATABASE_URL", "error", err)
	}

	db, err = pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalln("Unable to create connection pool", "error", err)
	}

	_, err = db.Exec(context.Background(), structureSQL)
	if err != nil {
		log.Fatalln(err, ", pg executing:", structureSQL)
	}

	http.HandleFunc("/favicon.ico", favHandler)
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/", refHandler)
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatalln(err)
	}
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
	_, err = db.Exec(r.Context(), `insert into ref(id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
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
	)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	http.Redirect(rw, r, dst, http.StatusTemporaryRedirect)
	return
}

func viewHandler(rw http.ResponseWriter, r *http.Request) {
	pin := r.URL.Query().Get("pin")
	if pin != authPin {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte(`add "pin" query param`))
		return
	}

	events := []ref{}
	rows, err := db.Query(r.Context(), "select (id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude) from ref")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	var refEvent ref
	for rows.Next() && err == nil {
		err = rows.Scan(
			&refEvent.ID,
			&refEvent.CreatedAt,
			&refEvent.Name,
			&refEvent.Dest,
			&refEvent.RequestAddr,
			&refEvent.UserAgent,
			&refEvent.Continent,
			&refEvent.Country,
			&refEvent.Region,
			&refEvent.City,
			&refEvent.Zip,
			&refEvent.Latitude,
			&refEvent.Longitiude,
		)
		events = append(events, refEvent)
		fmt.Printf("ref: %+v\n", refEvent)
	}
	if err != nil {
		http.Error(rw, fmt.Sprintf("e: %+v", refEvent)+",err: "+err.Error(), 500)
		return
	}
	resFmt := r.URL.Query().Get("fmt")
	var b []byte
	switch resFmt {
	case "csv":
		head := strings.Join([]string{"id", "created_at", "name", "dest", "request_addr", "continent", "country", "region", "city", "user_agent"}, ",")
		table := []string{head}
		for _, e := range events {
			e.UserAgent = strings.ReplaceAll(e.UserAgent, ",", ";")
			table = append(table, strings.Join([]string{e.ID, time.Unix(0, e.CreatedAt).Format(time.RFC3339), e.Name, e.Dest, e.RequestAddr, e.Continent, e.Country, e.Region, e.City, e.UserAgent}, ","))
		}
		b = []byte(strings.Join(table, "\n"))

	case "json":
		b, err = json.MarshalIndent(events, "", " ")
		if err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
	default:
		b, err = json.MarshalIndent(events, "", " ")
		if err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
	}
	rw.WriteHeader(200)
	rw.Write(b)
}

func favHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write(faviconFile)
}

type locInfo struct {
	Continent string  `json:"continent"`
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Zip       string  `json:"zip"`
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
}

func getLoc(ctx context.Context, addr string) (*locInfo, error) {
	url := "http://api.ipstack.com/" + addr + "?access_key=" + ipstackAPIKey
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	info := locInfo{}
	err = json.Unmarshal(b, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
