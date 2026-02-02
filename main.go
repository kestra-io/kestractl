package main

import (
	"os"

	"github.com/kestra-io/kestra-cli/src/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
