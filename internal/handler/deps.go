package handler

import "context"

type Store interface {
	Deposit(ctx context.Context, requestID, walletID, currency string, amount float64) error
	Withdraw(ctx context.Context, requestID, walletID, currency string, amount float64) error
	Transfer(ctx context.Context, requestID, fromWallet, toWallet, currency string, amount float64) error
	GetWalletBalance(ctx context.Context, walletID string) (float64, string, error)
}

type Publisher interface {
	PublishEvent(ctx context.Context, subject string, v interface{}) error
}
