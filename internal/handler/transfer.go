package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fundingpips/wallet-service/internal/nats"
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

func HandleTransfer(store *storage.Store, nc *nats.Client) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		var req TransferRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Println("transfer: bad payload:", err)
			return
		}
		if !validUUID(req.RequestID) || !validUUID(req.FromWalletID) || !validUUID(req.ToWalletID) || !validAmount(req.Amount) || !validCurrency(req.Currency) {
			publishFailed(nc, req.RequestID, "transfer", "invalid request")
			return
		}
		if req.FromWalletID == req.ToWalletID {
			publishFailed(nc, req.RequestID, "transfer", storage.ErrSameWallet.Error())
			return
		}

		if err := store.Transfer(context.Background(), req.RequestID, req.FromWalletID, req.ToWalletID, req.Currency, req.Amount); err != nil {
			fmt.Println("transfer failed:", err)
			publishFailed(nc, req.RequestID, "transfer", err.Error())
			return
		}

		publishCompleted(nc, req.RequestID, "transfer")
	}
}
