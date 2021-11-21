package main

import (
	"context"
	"crypto/md5"
	"embed"
	_ "embed"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	//go:embed embed
	embedFS embed.FS

	defaultDest    = os.Getenv("DEFAULT_DEST")
	salt           = os.Getenv("SALT")
	ipstackAPIKey  = os.Getenv("IPSTACK_API_KEY")
	hasher         = md5.New()
	defaultPinHash string
	db             *pgxpool.Pool
	locTimeout     time.Duration
)

func main() {
	rand.Seed(time.Now().UnixNano())
	if salt == "" {
		log.Fatalln("env var SALT is required")
	}
	_, err := hasher.Write([]byte(salt))
	if err != nil {
		log.Fatalln("could not initialize hasher:", err)
	}
	defaultPinHash = fmt.Sprintf("%x", hasher.Sum([]byte(os.Getenv("PIN")))[:3])

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

	structureSQL, err := embedFS.ReadFile("embed/structure.sql")
	if err != nil {
		log.Fatalln(err)
	}
	_, err = db.Exec(context.Background(), string(structureSQL))
	if err != nil {
		log.Fatalln(err, ", pg executing:", string(structureSQL))
	}

	if os.Getenv("REPAIR_DB") == "true" {
		if err := repairDB(db); err != nil {
			log.Fatalln("error repairing db:", err)
		}
	}

	locTimeout, err = time.ParseDuration(os.Getenv("LOC_TIMEOUT"))
	if err != nil {
		log.Println("setting loc timeout to default 300ms")
		locTimeout = 300 * time.Millisecond
	}

	http.HandleFunc("/favicon.ico", favHandler)
	http.HandleFunc("/robots.txt", robotsHandler)
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/info", infoHandler)
	http.HandleFunc("/pins/new", newPinsHandler)
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

func repairDB(db *pgxpool.Pool) error {
	log.Println("repairing db ...")
	_, err := db.Exec(
		context.Background(),
		"delete from ref where pin_hash = ''",
	)
	if err != nil {
		return err
	}

	rows, err := db.Query(
		context.Background(),
		`select id, request_addr
		from ref
		where zip = ''`,
	)

	needsGeo := make([]Event, 0)
	for rows.Next() {
		var id, addr string
		err := rows.Scan(&id, &addr)
		if err != nil {
			return err
		}
		needsGeo = append(needsGeo, Event{ID: id, RequestAddr: addr})
	}
	log.Println(len(needsGeo), "events need geo data")
	for _, ref := range needsGeo {
		info, err := checkCache(context.Background(), ref.RequestAddr)
		if err != nil {
			log.Println("could not repair record:", ref.ID, ref.RequestAddr)
			continue
		}
		_, err = db.Exec(
			context.Background(),
			`update ref 
				set continent = $1, 
				country = $2,
				region = $3,
				city = $4,
				zip = $5,
				latitude = $6,
				longitude = $7
				where id = $8`,
			info.Continent,
			info.Country,
			info.Region,
			info.City,
			info.Zip,
			info.Latitude,
			info.Longitude,
			ref.ID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
