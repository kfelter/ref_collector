package main

import (
	"context"
	"encoding/json"
	"io"
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
