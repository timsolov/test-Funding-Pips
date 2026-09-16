package storage

import "context"

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `SELECT pg_advisory_lock(872364)`); err != nil {
		return err
	}
	defer s.DB.ExecContext(context.Background(), `SELECT pg_advisory_unlock(872364)`)

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS wallets (
			wallet_id UUID PRIMARY KEY,
			balance NUMERIC(18,4) NOT NULL DEFAULT 0,
			currency VARCHAR(3) NOT NULL DEFAULT 'USD',
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id SERIAL PRIMARY KEY,
			request_id UUID,
			operation VARCHAR(20) NOT NULL,
			from_wallet UUID,
			to_wallet UUID,
			amount NUMERIC(18,4) NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`ALTER TABLE transactions ADD COLUMN IF NOT EXISTS reason TEXT`,
		`DELETE FROM transactions a USING transactions b WHERE a.request_id IS NOT NULL AND a.request_id = b.request_id AND a.id > b.id`,
		`CREATE UNIQUE INDEX IF NOT EXISTS transactions_request_id_uidx ON transactions (request_id)`,
		`ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_balance_non_negative`,
		`ALTER TABLE wallets ADD CONSTRAINT wallets_balance_non_negative CHECK (balance >= 0) NOT VALID`,
		`ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_amount_positive`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_amount_positive CHECK (amount > 0) NOT VALID`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
