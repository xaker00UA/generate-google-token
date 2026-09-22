package main

import (
	"context"
	"fmt"
	"os"

	"generate-google-cred/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
