package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/app"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/diagnostics"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("aimilivpn %s (%s, %s)\n", version, commit, date)
			return
		case "check":
			_ = checkCmd.Parse(os.Args[2:])
			cfg := config.Load(version)
			for _, check := range diagnostics.RuntimeChecks(cfg) {
				status := "ok"
				if !check.OK {
					status = "fail"
				}
				fmt.Printf("[%s] %s: %s\n", status, check.Name, check.Message)
			}
			return
		case "serve":
			_ = serveCmd.Parse(os.Args[2:])
		default:
			_ = serveCmd.Parse(os.Args[1:])
		}
	} else {
		_ = serveCmd.Parse(nil)
	}

	cfg := config.Load(version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.New(cfg)
	if err != nil {
		log.Fatalf("init failed: %v", err)
	}
	if err := runtime.Run(ctx); err != nil {
		log.Fatalf("runtime stopped: %v", err)
	}
}
