package main

import (
	"context"
	"os"

	"github.com/example/agent-cli-starter/internal/buildinfo"
	"github.com/example/agent-cli-starter/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	code := cli.Execute(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		cli.Options{
			Build: buildinfo.Info{
				Version: version,
				Commit:  commit,
				Date:    date,
			},
		},
	)
	os.Exit(code)
}
