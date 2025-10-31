package main

import (
	"os"

	"github.com/branchd-dev/branchd/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
