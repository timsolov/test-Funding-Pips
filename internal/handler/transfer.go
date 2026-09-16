package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
)

type TransferRequest struct {
	RequestID    string  `json:"request_id"`
	FromWalletID string  `json:"from_wallet_id"`
	ToWalletID   string  `json:"to_wallet_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
}

func HandleTransfer(ctx context.Context, store Store, nc Publisher) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		reqCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		var req TransferRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Println("transfer: bad payload:", err)
			return
		}
		if !validUUID(req.RequestID) || !validUUID(req.FromWalletID) || !validUUID(req.ToWalletID) || !validAmount(req.Amount) || !validCurrency(req.Currency) {
			publishFailed(reqCtx, nc, req.RequestID, "transfer", "invalid request")
			return
		}
		if req.FromWalletID == req.ToWalletID {
			publishFailed(reqCtx, nc, req.RequestID, "transfer", storage.ErrSameWallet.Error())
			return
		}

		if err := store.Transfer(reqCtx, req.RequestID, req.FromWalletID, req.ToWalletID, req.Currency, req.Amount); err != nil {
			fmt.Println("transfer failed:", err)
			publishFailed(reqCtx, nc, req.RequestID, "transfer", err.Error())
			return
		}

		publishCompleted(reqCtx, nc, req.RequestID, "transfer")
	}
}
