package main

import (
	"context"
	"fmt"
	"os"

	"github.com/actions/actions-sync/cmd"
)

func main() {
	ctx := context.Background()
	if err := cmd.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
