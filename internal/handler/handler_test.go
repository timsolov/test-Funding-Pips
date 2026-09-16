package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	walletnats "github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
)

func testStore(t *testing.T) *storage.Store {
	t.Helper()
	url := os.Getenv("PG_URL")
	if url == "" {
		url = "postgres://walletuser:walletpass@localhost:5432/wallet?sslmode=disable"
	}
	store, err := storage.NewStore(url)
	if err != nil {
		t.Fatalf("cannot connect to postgres, run docker-compose up -d nats postgres: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testNATS(t *testing.T) *walletnats.Client {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}
	nc, err := walletnats.Connect(url)
	if err != nil {
		t.Fatalf("cannot connect to nats, run docker-compose up -d nats postgres: %v", err)
	}
	t.Cleanup(func() { nc.Close() })
	return nc
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

func assertAmount(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDepositSameRequestIdIsNotAppliedTwice(t *testing.T) {
	store := testStore(t)
	nc := testNATS(t)
	walletID := newID()
	requestID := newID()

	payload, err := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  walletID,
		Amount:    50,
		Currency:  "USD",
	})
	if err != nil {
		t.Fatal(err)
	}

	h := HandleDeposit(store, nc)
	h(&natsgo.Msg{Data: payload})
	h(&natsgo.Msg{Data: payload})

	balance, _, err := store.GetWalletBalance(context.Background(), walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 50)
}

func TestBalanceAfterDepositUsesWallet(t *testing.T) {
	store := testStore(t)
	nc := testNATS(t)
	walletID := newID()

	if err := store.Deposit(context.Background(), newID(), walletID, "USD", 80); err != nil {
		t.Fatal(err)
	}

	if _, err := nc.Conn.Subscribe("wallet.balance.test."+walletID, HandleBalance(store)); err != nil {
		t.Fatal(err)
	}

	req, err := json.Marshal(BalanceRequest{WalletID: walletID})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := nc.Conn.Request("wallet.balance.test."+walletID, req, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var resp BalanceResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatal(err)
	}
	assertAmount(t, resp.Balance, 80)
}

func TestWithdrawNegativeAmountDoesNotChangeBalance(t *testing.T) {
	store := testStore(t)
	nc := testNATS(t)
	walletID := newID()
	if err := store.Deposit(context.Background(), newID(), walletID, "USD", 30); err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(WithdrawRequest{
		RequestID: newID(),
		WalletID:  walletID,
		Amount:    -10,
		Currency:  "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	HandleWithdraw(store, nc)(&natsgo.Msg{Data: payload})

	balance, _, err := store.GetWalletBalance(context.Background(), walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 30)
}

func TestTransferToSameWalletIsRejected(t *testing.T) {
	store := testStore(t)
	nc := testNATS(t)
	walletID := newID()
	requestID := newID()
	if err := store.Deposit(context.Background(), newID(), walletID, "USD", 20); err != nil {
		t.Fatal(err)
	}

	failed := make(chan *natsgo.Msg, 1)
	if _, err := nc.Conn.Subscribe("wallet.events.failed", func(msg *natsgo.Msg) {
		failed <- msg
	}); err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: walletID,
		ToWalletID:   walletID,
		Amount:       5,
		Currency:     "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	HandleTransfer(store, nc)(&natsgo.Msg{Data: payload})

	select {
	case msg := <-failed:
		var ev Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.RequestID != requestID {
			t.Fatalf("got request_id %s, want %s", ev.RequestID, requestID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected failed event for transfer to same wallet")
	}

	balance, _, err := store.GetWalletBalance(context.Background(), walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 20)
}

func TestBalanceMissingWalletReplies(t *testing.T) {
	store := testStore(t)
	nc := testNATS(t)
	walletID := newID()
	subject := "wallet.balance.missing." + walletID

	if _, err := nc.Conn.Subscribe(subject, HandleBalance(store)); err != nil {
		t.Fatal(err)
	}

	req, err := json.Marshal(BalanceRequest{WalletID: walletID})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := nc.Conn.Request(subject, req, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var resp map[string]string
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "wallet not found" {
		t.Fatalf("got %#v", resp)
	}
}
