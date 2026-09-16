package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/lib/pq"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrCurrencyMismatch  = errors.New("currency mismatch")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrSameWallet        = errors.New("cannot transfer to same wallet")
	ErrInvalidRequest    = errors.New("invalid request")
)

type Store struct {
	DB *sql.DB
}

func NewStore(ctx context.Context, pgURL string) (*Store, error) {
	db, err := sql.Open("postgres", pgURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	store := &Store{DB: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func validAmount(amount float64) bool {
	return amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

type ledgerRow struct {
	status string
	reason sql.NullString
}

func lookupRequest(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, requestID string) (*ledgerRow, error) {
	var row ledgerRow
	err := q.QueryRowContext(ctx, `SELECT status, reason FROM transactions WHERE request_id = $1`, requestID).Scan(&row.status, &row.reason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func errorFromReason(reason string) error {
	switch reason {
	case ErrInsufficientFunds.Error():
		return ErrInsufficientFunds
	case ErrWalletNotFound.Error():
		return ErrWalletNotFound
	case ErrCurrencyMismatch.Error():
		return ErrCurrencyMismatch
	case ErrInvalidAmount.Error():
		return ErrInvalidAmount
	case ErrSameWallet.Error():
		return ErrSameWallet
	case ErrInvalidRequest.Error():
		return ErrInvalidRequest
	default:
		return errors.New(reason)
	}
}

func replay(row *ledgerRow) error {
	if row.status == "completed" {
		return nil
	}
	if row.reason.Valid && row.reason.String != "" {
		return errorFromReason(row.reason.String)
	}
	return errors.New("operation failed")
}

func insertLedger(ctx context.Context, tx *sql.Tx, requestID, operation string, fromWallet, toWallet *string, amount float64, status, reason string) error {
	var reasonArg any
	if reason != "" {
		reasonArg = reason
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO transactions (request_id, operation, from_wallet, to_wallet, amount, status, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, requestID, operation, fromWallet, toWallet, amount, status, reasonArg)
	return err
}

func (s *Store) runOp(ctx context.Context, requestID string, apply func(*sql.Tx) error, record func(*sql.Tx, string, string) error) error {
	if requestID == "" {
		return ErrInvalidRequest
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existing, err := lookupRequest(ctx, tx, requestID)
	if err != nil {
		return err
	}
	if existing != nil {
		return replay(existing)
	}

	applyErr := apply(tx)
	status := "completed"
	reason := ""
	if applyErr != nil {
		status = "failed"
		reason = applyErr.Error()
	}

	if err := record(tx, status, reason); err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback()
			row, lookupErr := lookupRequest(ctx, s.DB, requestID)
			if lookupErr != nil {
				return lookupErr
			}
			if row != nil {
				return replay(row)
			}
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return applyErr
}

func (s *Store) GetWalletBalance(ctx context.Context, walletID string) (float64, string, error) {
	var balance float64
	var currency string
	err := s.DB.QueryRowContext(ctx, `SELECT balance, currency FROM wallets WHERE wallet_id = $1`, walletID).Scan(&balance, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrWalletNotFound
	}
	return balance, currency, err
}

func (s *Store) SumBalanceFromTransactions(ctx context.Context, walletID string) (float64, error) {
	var balance float64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(
			SUM(CASE WHEN to_wallet = $1 THEN amount ELSE 0 END) -
			SUM(CASE WHEN from_wallet = $1 THEN amount ELSE 0 END),
		0)
		FROM transactions WHERE (from_wallet = $1 OR to_wallet = $1) AND status = 'completed'
	`, walletID).Scan(&balance)
	return balance, err
}

func (s *Store) Deposit(ctx context.Context, requestID, walletID, currency string, amount float64) error {
	if !validAmount(amount) {
		return ErrInvalidAmount
	}
	if walletID == "" {
		return ErrInvalidRequest
	}
	if currency == "" {
		currency = "USD"
	}

	toWallet := walletID
	return s.runOp(ctx, requestID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO wallets (wallet_id, balance, currency)
			VALUES ($1, $2, $3)
			ON CONFLICT (wallet_id) DO UPDATE
			SET balance = wallets.balance + EXCLUDED.balance,
			    updated_at = NOW()
			WHERE wallets.currency = EXCLUDED.currency
		`, walletID, amount, currency)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrCurrencyMismatch
		}
		return nil
	}, func(tx *sql.Tx, status, reason string) error {
		return insertLedger(ctx, tx, requestID, "deposit", nil, &toWallet, amount, status, reason)
	})
}

func (s *Store) Withdraw(ctx context.Context, requestID, walletID, currency string, amount float64) error {
	if !validAmount(amount) {
		return ErrInvalidAmount
	}
	if walletID == "" {
		return ErrInvalidRequest
	}

	fromWallet := walletID
	return s.runOp(ctx, requestID, func(tx *sql.Tx) error {
		var currentCurrency string
		err := tx.QueryRowContext(ctx, `SELECT currency FROM wallets WHERE wallet_id = $1 FOR UPDATE`, walletID).Scan(&currentCurrency)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWalletNotFound
		}
		if err != nil {
			return err
		}
		if currency != "" && currency != currentCurrency {
			return ErrCurrencyMismatch
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE wallets
			SET balance = balance - $1, updated_at = NOW()
			WHERE wallet_id = $2 AND balance >= $1
		`, amount, walletID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrInsufficientFunds
		}
		return nil
	}, func(tx *sql.Tx, status, reason string) error {
		return insertLedger(ctx, tx, requestID, "withdraw", &fromWallet, nil, amount, status, reason)
	})
}

func getWalletForUpdate(ctx context.Context, tx *sql.Tx, walletID string) (float64, string, error) {
	var balance float64
	var currency string
	err := tx.QueryRowContext(ctx, `SELECT balance, currency FROM wallets WHERE wallet_id = $1 FOR UPDATE`, walletID).Scan(&balance, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrWalletNotFound
	}
	return balance, currency, err
}

func (s *Store) Transfer(ctx context.Context, requestID, fromWallet, toWallet, currency string, amount float64) error {
	if !validAmount(amount) {
		return ErrInvalidAmount
	}
	if fromWallet == "" || toWallet == "" {
		return ErrInvalidRequest
	}

	return s.runOp(ctx, requestID, func(tx *sql.Tx) error {
		if fromWallet == toWallet {
			return ErrSameWallet
		}

		first, second := fromWallet, toWallet
		if first > second {
			first, second = second, first
		}
		if _, _, err := getWalletForUpdate(ctx, tx, first); err != nil {
			return err
		}
		if _, _, err := getWalletForUpdate(ctx, tx, second); err != nil {
			return err
		}

		_, fromCurrency, err := getWalletForUpdate(ctx, tx, fromWallet)
		if err != nil {
			return err
		}
		_, toCurrency, err := getWalletForUpdate(ctx, tx, toWallet)
		if err != nil {
			return err
		}
		if fromCurrency != toCurrency {
			return ErrCurrencyMismatch
		}
		if currency != "" && currency != fromCurrency {
			return ErrCurrencyMismatch
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE wallets
			SET balance = balance - $1, updated_at = NOW()
			WHERE wallet_id = $2 AND balance >= $1
		`, amount, fromWallet)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrInsufficientFunds
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE wallets
			SET balance = balance + $1, updated_at = NOW()
			WHERE wallet_id = $2
		`, amount, toWallet); err != nil {
			return err
		}
		return nil
	}, func(tx *sql.Tx, status, reason string) error {
		return insertLedger(ctx, tx, requestID, "transfer", &fromWallet, &toWallet, amount, status, reason)
	})
}
