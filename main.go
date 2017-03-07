package main

import (
	"os"

	"github.com/elastic/beats/libbeat/beat"

	"github.com/jahor/rmqbeat/beater"
)

func main() {
	err := beat.Run("rmqbeat", "", beater.New)
	if err != nil {
		os.Exit(1)
	}
}
