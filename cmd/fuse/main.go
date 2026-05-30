// fuse — semantic merge driver binary entry point.
package main

import (
	"os"

	"github.com/tabladrum/grove-suite/fuse/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
