package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/app"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/service"
	"os"
	"os/signal"
	"path/filepath"
)

var version = "0.1.0-dev"

func main() {
	if e := execute(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func execute() error {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	flags := flag.NewFlagSet("ezgh-obs-agent-windows", flag.ContinueOnError)
	path := flags.String("config", filepath.Join(base, "EZGH", "ObsAgent", "config", "config.yaml"), "configuration path")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	op := "run"
	if flags.NArg() > 0 {
		op = flags.Arg(0)
	}
	if op == "version" {
		fmt.Println(version)
		return nil
	}
	run := func(ctx context.Context, ready func()) error {
		c, e := config.Load(*path)
		if e != nil {
			return e
		}
		return app.Run(ctx, c, ready)
	}
	if op == "run" {
		hosted, e := service.Host(run)
		if hosted || e != nil {
			return e
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		return run(ctx, nil)
	}
	if op == "validate" {
		f, e := os.Open(*path)
		if e != nil {
			return fmt.Errorf("cannot open configuration")
		}
		defer f.Close()
		_, e = config.Parse(f)
		if e == nil {
			fmt.Println("configuration valid")
		}
		return e
	}
	switch op {
	case "status", "doctor", "reload-credential":
		c, e := config.Load(*path)
		if e != nil {
			return e
		}
		r, e := diagnostics.Request(context.Background(), diagnostics.Path(c.StateDir), op)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	default:
		return fmt.Errorf("unknown command; use run, validate, status, doctor, reload-credential or version")
	}
}
