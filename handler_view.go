package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

func viewHandler(rw http.ResponseWriter, r *http.Request) {
	time.Local, _ = time.LoadLocation("America/New_York")

	pin := r.URL.Query().Get("pin")
	if pin == "" {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte(`add "pin" query param`))
		return
	}
	/*
		old: pinHash := fmt.Sprintf("%x", hasher.Sum([]byte(pin))[:3])
		pinHash is now the full md5 hash of the pin + salt to ensure users
		dont share traffic events
	*/
	pinHash := fmt.Sprintf("%x", hasher.Sum([]byte(pin)))

	var (
		fromUnixNano = time.Now().Add(-24 * time.Hour).UnixNano()
		toUnixNano   = time.Now().UnixNano()
	)

	tRange := r.URL.Query().Get("range")
	if tRange == "all" {
		fromUnixNano = int64(0)
		toUnixNano = int64(math.MaxInt64)
	}

	events, err := getEvents(
		r.Context(),
		fromUnixNano,
		toUnixNano,
		pinHash,
		r.URL.Query().Get("name"),
		r.URL.Query().Get("ip"),
	)
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

func getEvents(ctx context.Context, fromUnixNano, toUnixNano int64, pinHash, name, ip string) ([]Event, error) {
	time.Local, _ = time.LoadLocation("America/New_York")
	var (
		events = make([]Event, 0)
		rows   pgx.Rows
		err    error
	)
	rows, err = db.Query(ctx,
		`select id, created_at, name, dst, request_addr, user_agent, continent, country, region, city, zip, latitude, longitude
			from ref 
			where latitude is not null 
			and longitude is not null 
			and created_at > $1 
			and created_at < $2
			and pin_hash = $3
			and ($4 = '' or name = $4)
			and ($5 = '' or request_addr = $5);`, fromUnixNano, toUnixNano, pinHash, name, ip)

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
		events = append(events, Event{
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
