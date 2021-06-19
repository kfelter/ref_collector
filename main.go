package main

import (
	"context"
	_ "embed"
	"encoding/json"
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
	faviconFile []byte
	defaultDest = os.Getenv("DEFAULT_DEST")
	db          *pgxpool.Pool
)

type ref struct {
	ID          string `json:"id"`
	CreatedAt   int64  `json:"created_at"`
	Name        string `json:"name"`
	Dest        string `json:"dst"`
	RequestAddr string `json:"request_addr"`
	UserAgent   string `json:"user_agent"`
}

func main() {
	if defaultDest == "" {
		log.Fatalln("env var DEFAULT_DEST is required")
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

	_, err := db.Exec(r.Context(), `insert into ref(id, created_at, name, dst, request_addr, user_agent) values ($1, $2, $3, $4, $5, $6)`,
		uuid.New().String(),
		time.Now().UnixNano(),
		refName,
		dst,
		r.RemoteAddr,
		r.Header.Get("User-Agent"),
	)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	http.Redirect(rw, r, dst, http.StatusTemporaryRedirect)
	return
}

func viewHandler(rw http.ResponseWriter, r *http.Request) {
	events := []ref{}
	rows, err := db.Query(r.Context(), "select * from ref")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	for rows.Next() && err == nil {
		var refEvent ref
		err = rows.Scan(&refEvent.ID, &refEvent.CreatedAt, &refEvent.Name, &refEvent.Dest, &refEvent.RequestAddr, &refEvent.UserAgent)
		events = append(events, refEvent)
	}
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	resFmt := r.URL.Query().Get("fmt")
	var b []byte
	switch resFmt {
	case "csv":
		head := strings.Join([]string{"id", "created_at", "name", "dest", "request_addr", "user_agent"}, ",")
		table := []string{head}
		for _, e := range events {
			table = append(table, strings.Join([]string{e.ID, time.Unix(0, e.CreatedAt).Format(time.RFC3339), e.Name, e.Dest, e.RequestAddr, e.UserAgent}, ","))
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
