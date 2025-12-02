// Command specfmt refactors OpenAPI 3.0 YAML specs to extract inline response
// schemas into named components/schemas for better code generation.
package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/bilte-co/aeroapi-go/internal/specfmt"
)

type FormatCmd struct {
	Input   string `arg:"" name:"input" help:"OpenAPI YAML file to format" type:"existingfile"`
	Output  string `short:"o" help:"Output file (defaults to input for in-place)"`
	DryRun  bool   `help:"Do not write file, only validate and report changes"`
	Verbose bool   `short:"v" help:"Verbose logging"`
}

func (cmd *FormatCmd) Run() error {
	out := cmd.Output
	if out == "" {
		out = cmd.Input
	}
	return specfmt.FormatFile(cmd.Input, out, specfmt.Options{
		DryRun:  cmd.DryRun,
		Verbose: cmd.Verbose,
	})
}

type CLI struct {
	Format FormatCmd `cmd:"" help:"Refactor inline response schemas into components/schemas."`
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli,
		kong.Name("specfmt"),
		kong.Description("OpenAPI spec formatter for better code generation"),
		kong.UsageOnError(),
	)
	err := ctx.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
