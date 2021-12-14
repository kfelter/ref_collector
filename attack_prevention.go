package main

import "context"

func countRequests(ip string, t0, t1 int64) (int, error) {
	row := db.QueryRow(context.Background(),
		`select count(distinct id)
			from ref 
			where created_at > $1 
			and created_at < $2
			and request_addr = $3;`, t0, t1, ip)
	count := 0
	err := row.Scan(&count)
	return count, err
}
