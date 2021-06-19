package main

import (
	"context"
	_ "embed"
	"encoding/json"
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
	GeoData     string `json:"geo_data"`
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

	id := r.Header.Get("X-Request-Id")
	if id == "" {
		id = uuid.New().String()
	}
	addr := r.Header.Get("X-Forwarded-For")
	if ss := strings.Split(addr, ","); len(ss) > 1 {
		addr = ss[0]
	}
	_, err := db.Exec(r.Context(), `insert into ref(id, created_at, name, dst, request_addr, user_agent) values ($1, $2, $3, $4, $5, $6)`,
		id,
		time.Now().UnixNano(),
		refName,
		dst,
		addr,
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
	pin := r.URL.Query().Get("pin")
	if pin != os.Getenv("PIN") {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte(`add "pin" query param`))
		return
	}

	lookupGeo := r.URL.Query().Get("loc") != ""

	events := []ref{}
	rows, err := db.Query(r.Context(), "select * from ref")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	for rows.Next() && err == nil {
		var refEvent ref
		err = rows.Scan(&refEvent.ID, &refEvent.CreatedAt, &refEvent.Name, &refEvent.Dest, &refEvent.RequestAddr, &refEvent.UserAgent)
		if lookupGeo {
			refEvent.GeoData, err = getLoc(refEvent.RequestAddr)
			if err != nil {
				http.Error(rw, err.Error(), 500)
				return
			}
		} else {
			refEvent.GeoData = "nil"
		}
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
		head := strings.Join([]string{"id", "created_at", "name", "dest", "request_addr", "loc", "user_agent"}, ",")
		table := []string{head}
		for _, e := range events {
			e.UserAgent = strings.ReplaceAll(e.UserAgent, ",", ";")
			table = append(table, strings.Join([]string{e.ID, time.Unix(0, e.CreatedAt).Format(time.RFC3339), e.Name, e.Dest, e.RequestAddr, e.GeoData, e.UserAgent}, ","))
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

func getLoc(addr string) (string, error) {
	v := strings.Split(addr, ":")[0]
	url := "http://api.ipstack.com/" + v + "?access_key=" + os.Getenv("IPSTACK_API_KEY")
	log.Println("geo request", url)
	res, err := http.DefaultClient.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	info := struct {
		Contenent string `json:"continent_code"`
		Country   string `json:"country_code"`
		Region    string `json:"region_code"`
		City      string `json:"city"`
	}{}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(b, &info)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{info.Contenent, info.Country, info.Region, info.City}, "_"), nil
}
