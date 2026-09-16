package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
)

type DepositRequest struct {
	RequestID string  `json:"request_id"`
	WalletID  string  `json:"wallet_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
}

func HandleDeposit(ctx context.Context, store *storage.Store, nc *nats.Client) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		reqCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		var req DepositRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Println("deposit: bad payload:", err)
			return
		}
		if !validUUID(req.RequestID) || !validUUID(req.WalletID) || !validAmount(req.Amount) || !validCurrency(req.Currency) {
			publishFailed(reqCtx, nc, req.RequestID, "deposit", "invalid request")
			return
		}

		if err := store.Deposit(reqCtx, req.RequestID, req.WalletID, req.Currency, req.Amount); err != nil {
			fmt.Println("deposit failed:", err)
			publishFailed(reqCtx, nc, req.RequestID, "deposit", err.Error())
			return
		}

		publishCompleted(reqCtx, nc, req.RequestID, "deposit")
	}
}
