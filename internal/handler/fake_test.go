package handler

import (
	"context"
	"encoding/json"
	"testing"

	natsgo "github.com/nats-io/nats.go"
)

type fakeStore struct {
	deposits int
}

func (f *fakeStore) Deposit(context.Context, string, string, string, float64) error {
	f.deposits++
	return nil
}

func (f *fakeStore) Withdraw(context.Context, string, string, string, float64) error {
	return nil
}

func (f *fakeStore) Transfer(context.Context, string, string, string, string, float64) error {
	return nil
}

func (f *fakeStore) GetWalletBalance(context.Context, string) (float64, string, error) {
	return 0, "", nil
}

type fakePublisher struct {
	events int
}

func (f *fakePublisher) PublishEvent(context.Context, string, interface{}) error {
	f.events++
	return nil
}

func TestDepositBadJSONDoesNotTouchStore(t *testing.T) {
	store := &fakeStore{}
	pub := &fakePublisher{}
	HandleDeposit(context.Background(), store, pub)(&natsgo.Msg{Data: []byte(`{not json`)})
	if store.deposits != 0 {
		t.Fatalf("store was called %d times", store.deposits)
	}
	if pub.events != 0 {
		t.Fatalf("event was published %d times", pub.events)
	}
}

func TestDepositInvalidAmountPublishesFailed(t *testing.T) {
	store := &fakeStore{}
	pub := &fakePublisher{}
	payload, err := json.Marshal(DepositRequest{
		RequestID: newID(),
		WalletID:  newID(),
		Amount:    -5,
		Currency:  "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	HandleDeposit(context.Background(), store, pub)(&natsgo.Msg{Data: payload})
	if store.deposits != 0 {
		t.Fatalf("store was called %d times", store.deposits)
	}
	if pub.events != 1 {
		t.Fatalf("got %d events, want 1", pub.events)
	}
}
