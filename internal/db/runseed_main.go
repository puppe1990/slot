//go:build ignore

package main

import (
	"log"

	"github.com/puppe1990/amarra-cais/pkg/cais"

	"github.com/puppe1990/slot/internal/db"
	"github.com/puppe1990/slot/internal/store"
)

func main() {
	cfg := cais.Load()
	s, err := store.NewSQLiteStore(cfg.DBPath, cfg.Env)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := db.RunSeeds(s); err != nil {
		log.Fatal(err)
	}
}
