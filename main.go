package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg, tiers, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	store, err := openStore(cfg.StoragePath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	client, err := mautrix.NewClient(cfg.Matrix.Homeserver, id.UserID(cfg.Matrix.UserID), cfg.Matrix.AccessToken)
	if err != nil {
		log.Fatalf("create Matrix client: %v", err)
	}
	client.Store = store

	bot, err := newBot(client, store, cfg, tiers)
	if err != nil {
		log.Fatal(err)
	}

	syncer, ok := client.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		log.Fatal("unexpected Matrix syncer type")
	}
	// Ignore history on the first sync and messages from before the bot joins a room.
	syncer.OnSync(client.DontProcessOldEvents)
	syncer.OnEventType(event.EventMessage, bot.handleMessage)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.Info("starting Matrix mileage bot", "user_id", cfg.Matrix.UserID, "homeserver", cfg.Matrix.Homeserver)
	if err := client.SyncWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("Matrix sync failed: %v", err)
	}
}
