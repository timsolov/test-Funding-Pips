package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
)

type WithdrawRequest struct {
	RequestID string  `json:"request_id"`
	WalletID  string  `json:"wallet_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
}

func HandleWithdraw(ctx context.Context, store *storage.Store, nc *nats.Client) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		reqCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		var req WithdrawRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Println("withdraw: bad payload:", err)
			return
		}
		if !validUUID(req.RequestID) || !validUUID(req.WalletID) || !validAmount(req.Amount) || !validCurrency(req.Currency) {
			publishFailed(reqCtx, nc, req.RequestID, "withdraw", "invalid request")
			return
		}

		if err := store.Withdraw(reqCtx, req.RequestID, req.WalletID, req.Currency, req.Amount); err != nil {
			fmt.Println("withdraw failed:", err)
			publishFailed(reqCtx, nc, req.RequestID, "withdraw", err.Error())
			return
		}

		publishCompleted(reqCtx, nc, req.RequestID, "withdraw")
	}
}
