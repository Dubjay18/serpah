# Seraph Architecture

> double-entry ledger engine · Go microservices · ACID money

## Service Topology

```
CLIENT
  │
  ▼
[gateway :8080]  ← JWT validation, rate limiting, reverse proxy
  │
  ├── [auth     :8081]  ← users, bcrypt, JWT issuance, refresh tokens
  ├── [accounts :8082]  ← account lifecycle, delegates balance to ledger
  ├── [ledger   :8083]  ← ★ double-entry engine, the invariant lives here
  └── [payments :8084]  ← transfer state machine, outbox → RabbitMQ
```

## Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Money representation | `int64` kobo | No float rounding errors |
| Balance storage | Derived via SQL view | Eliminates drift bugs |
| Idempotency | Client-supplied key | Safe retries, no double-charges |
| Service isolation | Each service owns its DB | No cross-service joins |
| Async events | Transactional outbox | Guaranteed delivery without 2PC |
| Infrastructure | Docker Compose | `make up` boots everything in <60s |

## The Double-Entry Invariant

Every transaction in the ledger satisfies:

```
SUM(DEBIT amounts) = SUM(CREDIT amounts)
```

This is enforced at:
1. Application layer (`validateDoubleEntry` in ledger service)
2. Database constraint on `ledger_entries`
3. Integration tests (concurrent transfer storm)
