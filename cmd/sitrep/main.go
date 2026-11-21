// Command sitrep is the Go port of the sitrep shell scripts. Commands arrive phase by phase; see docs/GO-PORT.md.
package main

import (
	"fmt"
	"os"
)

const version = "0.3.0-port"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "sitrep: the Go port is not ready; use bin/sitrep")
	os.Exit(1)
}
