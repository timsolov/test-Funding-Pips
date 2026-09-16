package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fundingpips/wallet-service/internal/config"
	"github.com/fundingpips/wallet-service/internal/handler"
	walletnats "github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.NewStore(cfg.PGUrl)
	if err != nil {
		fmt.Println("failed to connect to postgres:", err)
		os.Exit(1)
	}
	defer store.Close()
	fmt.Println("connected to postgres")

	nc, err := walletnats.Connect(cfg.NATSUrl)
	if err != nil {
		fmt.Println("failed to connect to nats:", err)
		os.Exit(1)
	}
	defer nc.Close()
	fmt.Println("connected to nats")

	if _, err := nc.Conn.QueueSubscribe("wallet.deposit", "wallet-workers", handler.HandleDeposit(store, nc)); err != nil {
		fmt.Println("subscribe deposit failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.withdraw", "wallet-workers", handler.HandleWithdraw(store, nc)); err != nil {
		fmt.Println("subscribe withdraw failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.transfer", "wallet-workers", handler.HandleTransfer(store, nc)); err != nil {
		fmt.Println("subscribe transfer failed:", err)
		os.Exit(1)
	}
	if _, err := nc.Conn.QueueSubscribe("wallet.balance", "wallet-workers", handler.HandleBalance(store)); err != nil {
		fmt.Println("subscribe balance failed:", err)
		os.Exit(1)
	}

	fmt.Println("wallet-service is running")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("shutting down")
	if err := nc.Conn.Drain(); err != nil {
		fmt.Println("nats drain failed:", err)
	}
}
