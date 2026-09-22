package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaker00UA/generate-google-token/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
