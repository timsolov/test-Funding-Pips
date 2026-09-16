package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
)

type BalanceRequest struct {
	WalletID string `json:"wallet_id"`
}

type BalanceResponse struct {
	WalletID string  `json:"wallet_id"`
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

func HandleBalance(store *storage.Store) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		var req BalanceRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Println("balance: bad payload:", err)
			return
		}
		if !validUUID(req.WalletID) {
			data, _ := json.Marshal(map[string]string{
				"wallet_id": req.WalletID,
				"error":     "invalid request",
			})
			_ = msg.Respond(data)
			return
		}

		balance, currency, err := store.GetWalletBalance(context.Background(), req.WalletID)
		if err != nil {
			if errors.Is(err, storage.ErrWalletNotFound) {
				data, _ := json.Marshal(map[string]string{
					"wallet_id": req.WalletID,
					"error":     "wallet not found",
				})
				_ = msg.Respond(data)
				return
			}
			fmt.Println("balance: wallet lookup failed:", err)
			return
		}

		resp := BalanceResponse{
			WalletID: req.WalletID,
			Balance:  balance,
			Currency: currency,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			fmt.Println("balance: marshal failed:", err)
			return
		}
		if err := msg.Respond(data); err != nil {
			fmt.Println("balance: respond failed:", err)
		}
	}
}
