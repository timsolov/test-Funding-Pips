# Senior Backend Developer — Take-Home Assignment

## Overview

You've just inherited `wallet-service` — a Go service that manages user balances and processes
financial operations (deposits, withdrawals, transfers) via NATS messaging, with PostgreSQL as the
data store. The team that originally built it has moved on to other projects.

The service works — it starts, connects to NATS and PostgreSQL, and processes messages. It's also
been in production for a while, and it shows: a few different hands have touched it, deadlines got
tight in places, and it was never really revisited after it shipped.

Lately that's starting to show up as complaints. QA has flagged balance numbers that don't look
right after load testing. Support has fielded a ticket about a transfer where the money left one
account and never showed up in the other. Someone on the finance side asked, offhand, whether a
retried request could end up applied twice. Nobody has had the time to sit down and actually work
through it — that's you now.

**Time expectation:** 4–6 hours. We value quality of judgment and depth of analysis over quantity.

---

## System Description

### Wallet Service

- Listens on NATS subjects:
  - `wallet.deposit` — add funds to a wallet
  - `wallet.withdraw` — remove funds from a wallet
  - `wallet.transfer` — move funds between two wallets
  - `wallet.balance` — get current balance (request-reply)
- Stores data in PostgreSQL
- Publishes events to NATS:
  - `wallet.events.completed` — successful operation
  - `wallet.events.failed` — failed operation

### Message Schemas

**`wallet.deposit` / `wallet.withdraw`**

```json
{
  "request_id": "string (UUID)",
  "wallet_id": "string (UUID)",
  "amount": "number (> 0)",
  "currency": "string (e.g. USD)"
}
```

**`wallet.transfer`**

```json
{
  "request_id": "string (UUID)",
  "from_wallet_id": "string (UUID)",
  "to_wallet_id": "string (UUID)",
  "amount": "number (> 0)",
  "currency": "string (e.g. USD)"
}
```

**`wallet.balance`** (request-reply)

```json
// Request
{ "wallet_id": "string (UUID)" }

// Response
{ "wallet_id": "...", "balance": 150.00, "currency": "USD" }
```

**`wallet.events.completed` / `wallet.events.failed`**

```json
{
  "request_id": "string (UUID)",
  "operation": "deposit | withdraw | transfer",
  "status": "completed | failed",
  "reason": "string | null",
  "timestamp": "ISO 8601"
}
```

---

## Getting Started

### Prerequisites
- Go 1.22+
- Docker and Docker Compose

### Run Everything

```bash
docker-compose up -d --build
```

This starts:
- **NATS** on port `4222` (monitoring on `8222`)
- **PostgreSQL** on port `5432` (user: `walletuser`, password: `walletpass`, database: `wallet`)
- **wallet-service** — built from the `Dockerfile`, started once NATS and PostgreSQL are healthy

There are no pre-seeded wallets — `init.sql` only creates the schema. A wallet is created
automatically on its first deposit, so send a `wallet.deposit` for a fresh `wallet_id` to bring an
account into existence.

### Running the Service Locally (for development)

When you're actively changing the code it's usually quicker to run just the infrastructure in
Docker and run the service from your shell:

```bash
docker-compose up -d nats postgres
go run ./cmd/wallet-service
```

(Stop the containerised service first — `docker-compose stop wallet-service` — so you don't have two
instances processing the same subjects.)

The service reads its configuration from environment variables (see `.env` for defaults — `NATS_URL`,
`PG_URL`). Inside Compose these point at the `nats` and `postgres` service names; running locally they
fall back to `localhost`.

### Database Schema

Created on startup:

```sql
CREATE TABLE wallets (
    wallet_id UUID PRIMARY KEY,
    balance NUMERIC(18,4) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE transactions (
    id SERIAL PRIMARY KEY,
    request_id UUID,
    operation VARCHAR(20) NOT NULL,
    from_wallet UUID,
    to_wallet UUID,
    amount NUMERIC(18,4) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
```

---

## Your Task

This is deliberately open-ended. We are not going to tell you what to look for, what's wrong, or how
to go about it — working that out for yourself, and deciding what to do about it, is the exercise.

Spend real time with the service and form your own view of where it stands. Then do whatever you
judge to be the right thing to do about it.

Submit your work with a README explaining what you found, what you decided to do, and the reasoning
behind those choices. Commit as you go rather than dropping everything in one final commit.

---

## What We Evaluate

| Area | What we look for |
|---|---|
| **Problem diagnosis** | How much of what's actually wrong you uncover, including the less obvious issues |
| **Judgment & prioritization** | Whether your read on severity and your choice of approach make sense for a live financial system on a time box |
| **Fix quality** | Correctness, simplicity, and production-readiness of whatever you actually ship |
| **Financial thinking** | Whether you reason about correctness the way a system handling real money demands |
| **Testing** | Tests that prove a problem was real and that your change actually closes it |
| **Code quality** | Clean Go code, error handling, naming, structure |
| **Communication** | A clear README explaining what you found, what you decided, and why |

---

## Deliverables

- A repository (or archive) with your changes to the service
- Tests that demonstrate the problems you identified were real and that your changes address them
- A README with your findings, your approach, and your reasoning
- The code should run against the provided docker-compose setup

---

## Notes

- You have full latitude over the code — keep the structure as it is or change it however you see
  fit, as long as you explain any significant decisions in your README.
- We will review your commit history alongside the final result, so keep it legible.
- If you run out of time, be clear about what you didn't get to and why — that's more useful to us
  than a rushed attempt at everything.
- If something about the intended behavior is ambiguous, document your interpretation and
  assumptions in your README rather than guessing what we want.

---

## My work

This is the original wallet-service, I almost did not touch it yet. I only fixed `.gitignore` because `wallet-service` was ignoring `cmd/wallet-service` and the main file was not going into git.

I will change things in small commits. For every change I write here why I did it.

### Tests first

I added tests before I change the service. They show the real money bugs that people was complaining about:

- many withdraws at the same time can make balance negative
- transfer to a wallet that not exist still takes money from sender
- same `request_id` can be applied two times
- balance is calculated from `transactions`, so it can be wrong if ledger is not updated
- negative withdraw can add money to the wallet

This tests need postgres and nats (`docker-compose up -d nats postgres`), then `go test ./...`.

Right now tests fail. That is expected, because the bugs are still in the code. After I fix it, same tests should pass. This is how I prove the problem was real and that the fix actually works.

### Database constraints

Code can have bugs, so I also put rules in postgres:

- `request_id` must be unique. This is needed later for retries, so the same request cannot be saved two times.
- wallet `balance` cannot go below 0
- transaction `amount` must be bigger than 0

The service now runs this on start, so it works even if postgres volume already exist. If old rows have the same `request_id` two times (because of the bug), migrate keeps the oldest row. I use `NOT VALID` for check constraints so old bad rows (from tests) do not block migrate. New writes still cannot break the rules. This is not the full fix yet, it is only extra safety in the database.

### Atomic operations

This is the main money fix.

Deposit, withdraw and transfer now run in one postgres transaction. The ledger row is written in the same transaction as the balance change, so they cannot go out of sync.

For withdraw I don't do select-then-update anymore. I do `UPDATE ... WHERE balance >= amount`. If two withdraws come at the same time, only one can take the last money.

Transfer lock both wallets (`FOR UPDATE`) in a stable order, so we don't get deadlocks. If the destination wallet does not exist, the whole transfer is cancelled and sender keep the money. This was the support ticket.

Same `request_id` is checked before we move money. If we already finished this request, we return the same result and we do not apply it again. Finance asked about this.


### Validate requests and read the real balance

I check UUID, amount `> 0` and currency (3 uppercase letters, or empty = USD) before we touch the database. Transfer to the same wallet is rejected.

`wallet.balance` now reads `wallets.balance`. This is the operational balance. If wallet does not exist, we still reply (`error: wallet not found`) so the client is not waiting forever.

I also use a NATS queue group `wallet-workers`, so two instances dont process the same message. Subscribe errors stop the process. On shutdown I drain NATS connections.

Completed / failed events are published only after the database commit.

### Graceful shutdown and context

Before, handlers used `context.Background()`, so SIGTERM cannot stop in-flight SQL. Now `main` use `signal.NotifyContext` and pass this context down: postgres ping/migrate, nats connect, and every handler.

Each message get a 5s timeout from that parent context. On shutdown we drain NATS (with 10s timeout) and then close postgres. If drain is too slow we force close.

### Accept interfaces, return structs

Handlers now take `Store` and `Publisher` interfaces, not `*storage.Store` and `*nats.Client`. `main` still create the real structs (`NewStore`, `Connect`) and pass them in. This is the Go rule: accept interfaces, return structs. It also make tests easier later, we can fake store or nats without changing handlers.

### Rollback money if the operation fail

If deposit/withdraw/transfer fail after some SQL already ran, I do not commit that. The money transaction is rolled back, and the failed row is saved in a new transaction. So a broken transfer cannot keep the debit and mark the request as failed at the same time.

### More tests

I added tests for a normal transfer, not enough money, currency mismatch, and same `request_id` with a different amount. Different payload with the same id is now an error, money is not moved. I also added small handler tests with a fake store, so bad JSON does not call the database.
