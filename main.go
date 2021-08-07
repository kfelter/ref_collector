package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/pkg/errors"
)

var (
	//go:embed structure.sql
	structureSQL string
	//go:embed favicon.ico
	faviconFile []byte
	//go:embed tmpl/map.html
	view_map_tmpl  string
	defaultDest    = os.Getenv("DEFAULT_DEST")
	authPin        = os.Getenv("PIN")
	neutrinoAPIKey = os.Getenv("NEUTRINO_API_KEY")
	ipstackAPIKey  = os.Getenv("IPSTACK_API_KEY")
	jwtKey         = []byte(os.Getenv("JWT_KEY"))
	db             *pgxpool.Pool
	locTimeout     time.Duration
)

type event struct {
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
	http.HandleFunc("/robots.txt", robotsHandler)
	http.HandleFunc("/view/map", viewMapHandler)
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/auth/token", tokenHandler)
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
	time.Local, _ = time.LoadLocation("America/New_York")

	if token := r.URL.Query().Get("token"); token != "" {
		http.SetCookie(rw, &http.Cookie{
			Name:  "Bearer",
			Value: token,
		})
	}

	c, err := r.Cookie("Bearer")
	if err != nil {
		pin := r.URL.Query().Get("pin")
		if pin != authPin {
			rw.WriteHeader(http.StatusUnauthorized)
			rw.Write([]byte(`add "pin" query param`))
			return
		}
	} else {
		claims, err := parseToken(c.Value)
		if err != nil {
			http.Error(rw, err.Error(), 401)
			return
		} else {
			fmt.Printf("user %s viewing refs", claims["user"])
		}
	}

	var (
		fromUnixNano = time.Now().Add(-24 * time.Hour).UnixNano()
		toUnixNano   = time.Now().UnixNano()
	)

	tRange := r.URL.Query().Get("range")
	if tRange == "all" {
		fromUnixNano = int64(0)
		toUnixNano = int64(math.MaxInt64)
	}

	events, err := getEvents(r.Context(), fromUnixNano, toUnixNano)
	if err != nil {
		http.Error(rw, err.Error(), 500)
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

func tokenHandler(w http.ResponseWriter, req *http.Request) {
	user := req.URL.Query().Get("user")
	pass := req.URL.Query().Get("pass")

	token, err := genToken(user, pass, jwtKey)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:  "Bearer",
		Value: token,
	})
	http.Redirect(w, req, "/view?token="+token, 302)
}

func genToken(user, pass string, key []byte) (string, error) {
	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user": user,
		"nbf":  time.Now().Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString(key)
}

func parseToken(tokenString string) (jwt.MapClaims, error) {
	// Parse takes the token string and a function for looking up the key. The latter is especially
	// useful if you use multiple keys for your application.  The standard is to use 'kid' in the
	// head of the token to identify which key to use, but the parsed token (head and claims) is provided
	// to the callback, providing flexibility.
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
		return jwtKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("token not valid")
	}

	var claims jwt.MapClaims
	var ok bool
	if claims, ok = token.Claims.(jwt.MapClaims); !ok {
		return nil, fmt.Errorf("error getting claims")
	}
	return claims, nil

}

func getEvents(ctx context.Context, fromUnixNano, toUnixNano int64) ([]event, error) {
	time.Local, _ = time.LoadLocation("America/New_York")
	events := []event{}
	rows, err := db.Query(ctx,
		`select id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude
		 from ref 
		 where latitude is not null 
		 and longitude is not null 
		 and created_at > $1 
		 and created_at < $2`, fromUnixNano, toUnixNano)
	if err != nil {
		return nil, errors.Wrap(err, "db.Query")
	}
	var (
		ID          = new(string)
		CreatedAt   = new(int64)
		Name        = new(string)
		Dest        = new(string)
		RequestAddr = new(string)
		UserAgent   = new(string)
		Continent   = new(sql.NullString)
		Country     = new(sql.NullString)
		Region      = new(sql.NullString)
		City        = new(sql.NullString)
		Zip         = new(sql.NullString)
		Latitude    = new(sql.NullFloat64)
		Longitude   = new(sql.NullFloat64)
	)
	for rows.Next() && err == nil {
		err = rows.Scan(
			ID,
			CreatedAt,
			Name,
			Dest,
			RequestAddr,
			UserAgent,
			Continent,
			Country,
			Region,
			City,
			Zip,
			Latitude,
			Longitude,
		)
		if err != nil {
			return nil, errors.Wrap(err, "scan")
		}
		events = append(events, event{
			ID:          *ID,
			CreatedAt:   *CreatedAt,
			Name:        *Name,
			Dest:        *Dest,
			RequestAddr: *RequestAddr,
			UserAgent:   *UserAgent,
			Continent:   Continent.String,
			Country:     Country.String,
			Region:      Region.String,
			City:        City.String,
			Zip:         Zip.String,
			Latitude:    Latitude.Float64,
			Longitude:   Longitude.Float64,
			TimeHuman:   time.Unix(0, *CreatedAt).Local().Format(time.RFC3339),
		})
	}
	return events, nil
}

func viewMapHandler(rw http.ResponseWriter, r *http.Request) {
	pin := r.URL.Query().Get("pin")
	if pin != authPin {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte(`add "pin" query param`))
		return
	}

	var (
		fromUnixNano = time.Now().Add(-24 * time.Hour).UnixNano()
		toUnixNano   = time.Now().UnixNano()
	)

	tRange := r.URL.Query().Get("range")
	if tRange == "all" {
		fromUnixNano = int64(0)
		toUnixNano = int64(math.MaxInt64)
	}

	events, err := getEvents(r.Context(), fromUnixNano, toUnixNano)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	t, err := template.New("map").Parse(view_map_tmpl)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	err = t.ExecuteTemplate(rw, "map", events)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
}

func favHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write(faviconFile)
}

type locInfo struct {
	Continent string  `json:"continent_name"`
	Country   string  `json:"country_name"`
	Region    string  `json:"region_name"`
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

func robotsHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte(`crawl-delay: 86400`))
}
