package main

import (
	"log"
)

func main() {
	log.Println("lynxd starting...")
	select {} // keep alive
}
