package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

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
	info, err := checkCache(ctx, addr)
	if err == nil {
		return &info, nil
	} else {
		log.Println("error checking ip locating cache:", err)
	}

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
	err = json.Unmarshal(b, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func checkCache(ctx context.Context, addr string) (locInfo, error) {
	info := locInfo{}
	row := db.QueryRow(ctx, "select continent, country, region, city, zip, latitude, longitude FROM ref where request_addr = $1", addr)
	err := row.Scan(&info.Continent, &info.Country, &info.Region, &info.City, &info.Zip, &info.Latitude, &info.Longitude)
	return info, err
}
