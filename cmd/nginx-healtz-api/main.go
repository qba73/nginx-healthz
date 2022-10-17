package main

import (
	"log"

	nginxhealthz "github.com/qba73/nginx-healthz"
)

func main() {
	if err := nginxhealthz.RunServer(); err != nil {
		log.Fatal(err)
	}
}
