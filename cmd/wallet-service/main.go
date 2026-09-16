package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fundingpips/wallet-service/internal/config"
	"github.com/fundingpips/wallet-service/internal/handler"
	walletnats "github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := storage.NewStore(ctx, cfg.PGUrl)
	if err != nil {
		fmt.Println("failed to connect to postgres:", err)
		os.Exit(1)
	}
	defer store.Close()
	fmt.Println("connected to postgres")

	nc, err := walletnats.Connect(ctx, cfg.NATSUrl)
	if err != nil {
		fmt.Println("failed to connect to nats:", err)
		os.Exit(1)
	}
	fmt.Println("connected to nats")

	if _, err := nc.Conn.QueueSubscribe("wallet.deposit", "wallet-workers", handler.HandleDeposit(ctx, store, nc)); err != nil {
		fmt.Println("subscribe deposit failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.withdraw", "wallet-workers", handler.HandleWithdraw(ctx, store, nc)); err != nil {
		fmt.Println("subscribe withdraw failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.transfer", "wallet-workers", handler.HandleTransfer(ctx, store, nc)); err != nil {
		fmt.Println("subscribe transfer failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.balance", "wallet-workers", handler.HandleBalance(ctx, store)); err != nil {
		fmt.Println("subscribe balance failed:", err)
		os.Exit(1)
	}

	fmt.Println("wallet-service is running")
	<-ctx.Done()
	fmt.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	drained := make(chan error, 1)
	go func() {
		drained <- nc.Drain()
	}()
	select {
	case err := <-drained:
		if err != nil {
			fmt.Println("nats drain failed:", err)
			nc.Close()
		}
	case <-shutdownCtx.Done():
		fmt.Println("nats drain timeout")
		nc.Close()
	}
}
