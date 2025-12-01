package main

import (
	"log"

	"github.com/1l0/gnak/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
