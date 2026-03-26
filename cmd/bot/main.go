package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"tlmsc/internal/config"
	"tlmsc/internal/download"
	"tlmsc/internal/streamrip"
	"tlmsc/internal/telegram"
)

func main() {
	// Load configuration
	cfg := config.LoadFromEnv()

	if cfg.TelegramToken == "" {
		fmt.Println("Error: TELEGRAM_BOT_TOKEN environment variable not set")
		os.Exit(1)
	}

	if cfg.DebugMode {
		fmt.Println("Debug mode enabled")
		fmt.Printf("Staging path: %s\n", cfg.StagingPath)
	}

	// Initialize clients
	streamripClient := streamrip.NewClient(cfg.DebugMode)
	if !streamripClient.CheckStreamripInstalled() {
		fmt.Println("Error: streamrip is not installed. Please install it with: pip install streamrip")
		os.Exit(1)
	}

	// Initialize bot
	tgBot, err := telegram.NewBot(cfg.TelegramToken, cfg.DebugMode)
	if err != nil {
		fmt.Printf("Error initializing bot: %v\n", err)
		os.Exit(1)
	}

	me, err := tgBot.GetMe()
	if err != nil {
		fmt.Printf("Error getting bot info: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Bot @%s started successfully\n", me.UserName)

	// Initialize download queue
	downloadQueue := download.NewQueue(10)

	// Initialize handlers
	handlers := telegram.NewHandlers(tgBot, streamripClient, downloadQueue, cfg.StagingPath, cfg.DebugMode)

	// Register command handlers
	tgBot.RegisterHandler("start", handlers.HandleStart)
	tgBot.RegisterHandler("help", handlers.HandleHelp)
	tgBot.RegisterHandler("search", handlers.HandleSearch)
	tgBot.RegisterHandler("queue", handlers.HandleQueue)
	tgBot.RegisterHandler("import", handlers.HandleImport)

	// Register command menu in Telegram
	if err := tgBot.SetCommands(); err != nil {
		fmt.Printf("[telegram] Failed to set command menu: %v\n", err)
	}

	// Register callback handler
	tgBot.RegisterCallbackHandler(handlers.HandleCallback)

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel for signaling shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	// Create progress callback for download worker
	progressCallback := func(job *streamrip.DownloadJob, progress streamrip.Progress) {
		fmt.Printf("[download] Job %s: %s (%d%%)\n", job.ID, progress.Status, progress.Percent)
		handlers.UpdateDownloadMessage(job, progress)
	}

	// Start download worker
	downloadWorker := download.NewWorker(downloadQueue, streamripClient, progressCallback, cfg.DebugMode)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := downloadWorker.ProcessQueue(ctx); err != nil && err != context.Canceled {
			fmt.Printf("Download worker error: %v\n", err)
		}
	}()

	// Start telegram bot
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := tgBot.Start(ctx); err != nil && err != context.Canceled {
			fmt.Printf("Bot error: %v\n", err)
		}
	}()

	fmt.Println("Bot is running. Press Ctrl+C to exit.")

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down...")

	// Cancel all goroutines
	cancel()
	downloadQueue.Close()

	// Wait for all goroutines to finish
	wg.Wait()

	fmt.Println("Bot stopped gracefully")
}
