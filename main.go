package main

import (
	"os"

	"github.com/jahor/rmqbeat/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
