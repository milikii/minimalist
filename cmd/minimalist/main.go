package main

import (
	"fmt"
	"os"

	"minimalist/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "minimalist: %v\n", err)
		os.Exit(1)
	}
}
