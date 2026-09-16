package storage

import (
	"context"
	"errors"
	"testing"
)

func TestTransferSuccess(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	from := newID()
	to := newID()
	if err := store.Deposit(ctx, newID(), from, "USD", 100); err != nil {
		t.Fatal(err)
	}
	if err := store.Deposit(ctx, newID(), to, "USD", 20); err != nil {
		t.Fatal(err)
	}

	if err := store.Transfer(ctx, newID(), from, to, "USD", 30); err != nil {
		t.Fatal(err)
	}

	fromBal, _, err := store.GetWalletBalance(ctx, from)
	if err != nil {
		t.Fatal(err)
	}
	toBal, _, err := store.GetWalletBalance(ctx, to)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, fromBal, 70)
	assertAmount(t, toBal, 50)
}

func TestWithdrawInsufficientFunds(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	if err := store.Deposit(ctx, newID(), walletID, "USD", 10); err != nil {
		t.Fatal(err)
	}

	err := store.Withdraw(ctx, newID(), walletID, "USD", 50)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("got %v, want %v", err, ErrInsufficientFunds)
	}

	balance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 10)
}

func TestDepositCurrencyMismatch(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	if err := store.Deposit(ctx, newID(), walletID, "USD", 25); err != nil {
		t.Fatal(err)
	}

	err := store.Deposit(ctx, newID(), walletID, "EUR", 10)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("got %v, want %v", err, ErrCurrencyMismatch)
	}

	balance, currency, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	if currency != "USD" {
		t.Fatalf("got currency %s, want USD", currency)
	}
	assertAmount(t, balance, 25)
}

func TestSameRequestIdDifferentAmountIsRejected(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	requestID := newID()

	if err := store.Deposit(ctx, requestID, walletID, "USD", 50); err != nil {
		t.Fatal(err)
	}
	err := store.Deposit(ctx, requestID, walletID, "USD", 80)
	if !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("got %v, want %v", err, ErrRequestMismatch)
	}

	balance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 50)
}

func TestFailedRequestIdDoesNotRetryLater(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	requestID := newID()
	if err := store.Deposit(ctx, newID(), walletID, "USD", 10); err != nil {
		t.Fatal(err)
	}

	err := store.Withdraw(ctx, requestID, walletID, "USD", 40)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("got %v, want %v", err, ErrInsufficientFunds)
	}

	if err := store.Deposit(ctx, newID(), walletID, "USD", 100); err != nil {
		t.Fatal(err)
	}

	err = store.Withdraw(ctx, requestID, walletID, "USD", 40)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("retry got %v, want %v", err, ErrInsufficientFunds)
	}

	balance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 110)
}
