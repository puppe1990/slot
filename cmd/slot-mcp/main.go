// Command slot-mcp exposes the Slot scheduling app to MCP clients over stdio.
//
//	SLOT_URL=https://slot.apps.gestaobem.com SLOT_TOKEN=... slot-mcp
//
// stdout is the protocol; diagnostics go to stderr.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/puppe1990/slot/internal/mcpserver"
	"github.com/puppe1990/slot/internal/slotcli"
)

const version = "0.1.0"

func main() {
	token := os.Getenv("SLOT_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "slot-mcp: SLOT_TOKEN is required (the app's ADMIN_TOKEN)")
		os.Exit(1)
	}
	baseURL := os.Getenv("SLOT_URL")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	server := mcpserver.New(slotcli.New(baseURL, token), version)
	err := server.Run(ctx, os.Stdin, os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "slot-mcp:", err)
		os.Exit(1)
	}
}
