package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("PG_URL")
	if url == "" {
		url = "postgres://walletuser:walletpass@localhost:5432/wallet?sslmode=disable"
	}
	store, err := NewStore(context.Background(), url)
	if err != nil {
		t.Fatalf("cannot connect to postgres, run docker-compose up -d nats postgres: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
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

func TestConcurrentWithdrawDoesNotOverdraw(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	for round := 0; round < 8; round++ {
		walletID := newID()
		if err := store.Deposit(ctx, newID(), walletID, "USD", 100); err != nil {
			t.Fatal(err)
		}

		const workers = 20
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				_ = store.Withdraw(ctx, newID(), walletID, "USD", 10)
			}()
		}
		wg.Wait()

		balance, _, err := store.GetWalletBalance(ctx, walletID)
		if err != nil {
			t.Fatal(err)
		}
		if balance < 0 {
			t.Fatalf("balance went negative: %v", balance)
		}
		assertAmount(t, balance, 0)
	}
}

func TestTransferToMissingWalletDoesNotLoseMoney(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	from := newID()
	to := newID()
	if err := store.Deposit(ctx, newID(), from, "USD", 100); err != nil {
		t.Fatal(err)
	}

	err := store.Transfer(ctx, newID(), from, to, "USD", 40)
	if err == nil {
		t.Fatal("expected error when destination wallet does not exist")
	}

	balance, _, err := store.GetWalletBalance(ctx, from)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 100)
}

func TestNegativeWithdrawIsRejected(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	if err := store.Deposit(ctx, newID(), walletID, "USD", 50); err != nil {
		t.Fatal(err)
	}

	if err := store.Withdraw(ctx, newID(), walletID, "USD", -10); err == nil {
		t.Fatal("expected error for negative amount")
	}

	balance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 50)
}

func TestBalanceMatchesWalletAfterDepositWithoutLedger(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	if err := store.Deposit(ctx, newID(), walletID, "USD", 75); err != nil {
		t.Fatal(err)
	}

	walletBalance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	ledgerBalance, err := store.SumBalanceFromTransactions(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(walletBalance-ledgerBalance) > 0.0001 {
		t.Fatalf("wallet balance %v and ledger balance %v are different", walletBalance, ledgerBalance)
	}
}

func TestSameRequestIdIsNotAppliedTwice(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	walletID := newID()
	requestID := newID()

	if err := store.Deposit(ctx, requestID, walletID, "USD", 50); err != nil {
		t.Fatal(err)
	}
	if err := store.Deposit(ctx, requestID, walletID, "USD", 50); err != nil {
		t.Fatal(err)
	}

	balance, _, err := store.GetWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, balance, 50)
}

func TestTransferRollbackIfCreditFails(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	from := newID()
	to := newID()
	if err := store.Deposit(ctx, newID(), from, "USD", 100); err != nil {
		t.Fatal(err)
	}
	if err := store.Deposit(ctx, newID(), to, "USD", 10); err != nil {
		t.Fatal(err)
	}

	fn := "fail_credit_" + strings.ReplaceAll(to, "-", "")
	trg := "trg_" + strings.ReplaceAll(to, "-", "")
	_, err := store.DB.ExecContext(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'credit failed';
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s
		BEFORE UPDATE ON wallets
		FOR EACH ROW
		WHEN (OLD.wallet_id = '%s'::uuid)
		EXECUTE FUNCTION %s();
	`, fn, trg, to, fn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.DB.ExecContext(context.Background(), fmt.Sprintf(
			`DROP TRIGGER IF EXISTS %s ON wallets; DROP FUNCTION IF EXISTS %s();`, trg, fn,
		))
	})

	if err := store.Transfer(ctx, newID(), from, to, "USD", 40); err == nil {
		t.Fatal("expected credit to fail")
	}

	fromBal, _, err := store.GetWalletBalance(ctx, from)
	if err != nil {
		t.Fatal(err)
	}
	toBal, _, err := store.GetWalletBalance(ctx, to)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, fromBal, 100)
	assertAmount(t, toBal, 10)
}
