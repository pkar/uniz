package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pkar/uniz"
)

var version = "dev"

func main() {
	runner := uniz.New(uniz.Config{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Version: version})
	err := runner.Run(context.Background(), os.Args[1:])
	var exit *uniz.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.Err != nil) {
		fmt.Fprintln(os.Stderr, "uniz:", err)
	}
	os.Exit(uniz.ExitCode(err))
}
