package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	res, err := http.Get("https://ref-collector-2021.herokuapp.com/view?pin=" + os.Getenv("PIN"))
	if err != nil {
		log.Fatalln(err)
	}
	defer res.Body.Close()
	_, err = io.Copy(os.Stdout, res.Body)
	if err != nil {
		fmt.Println("copy to stdout:", err.Error())
	}
}
