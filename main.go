package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	//go:embed embed
	embedFS embed.FS

	defaultDest    = os.Getenv("DEFAULT_DEST")
	authPin        = os.Getenv("PIN")
	neutrinoAPIKey = os.Getenv("NEUTRINO_API_KEY")
	ipstackAPIKey  = os.Getenv("IPSTACK_API_KEY")
	jwtKey         = []byte(os.Getenv("JWT_KEY"))
	db             *pgxpool.Pool
	locTimeout     time.Duration
)

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

	structureSQL, err := embedFS.ReadFile("embed/structure.sql")
	if err != nil {
		log.Fatalln(err)
	}
	_, err = db.Exec(context.Background(), string(structureSQL))
	if err != nil {
		log.Fatalln(err, ", pg executing:", string(structureSQL))
	}

	http.HandleFunc("/favicon.ico", favHandler)
	http.HandleFunc("/robots.txt", robotsHandler)
	http.HandleFunc("/view/map", viewMapHandler)
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/", refHandler)
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatalln(err)
	}
}

func favHandler(rw http.ResponseWriter, r *http.Request) {
	data, err := embedFS.ReadFile("embed/favicon.ico")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	rw.WriteHeader(200)
	rw.Write(data)
}

func robotsHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte(`crawl-delay: 86400`))
}
