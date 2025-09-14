package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"youtube-bot-simple/internal/config"
	"youtube-bot-simple/internal/downloader"
	"youtube-bot-simple/internal/files"
	"youtube-bot-simple/internal/queue"
	"youtube-bot-simple/internal/state"
	"youtube-bot-simple/internal/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := files.EnsureDir(cfg.DownloadDir); err != nil {
		log.Fatalf("failed to ensure download dir: %v", err)
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("failed to init bot api: %v", err)
	}
	api.Debug = false
	log.Printf("[bot] authorized on account %s", api.Self.UserName)

	st := state.NewStore()
	q := queue.NewQueue(cfg.QueueCapacity, cfg.Concurrency)
	dl := downloader.NewRunner(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// GC временного стора и очистка файлов
	st.StartGC(ctx, 5*time.Minute)
	files.StartCleanup(ctx, cfg.DownloadDir, cfg.CleanupTTLHours)

	b := telegram.NewBot(api, cfg, st, q, dl)

	// запуск воркеров очереди
	q.Start(ctx, b.Worker)

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("[bot] shutting down...")
		cancel()
	}()

	if err := b.Start(ctx); err != nil {
		log.Printf("[bot] stopped: %v", err)
	}
}
