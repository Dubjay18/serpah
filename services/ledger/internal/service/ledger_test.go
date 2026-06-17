package service_test

import (
	"context"
	"testing"

	"github.com/Dubjay18/seraph/services/ledger/internal/service"
	"github.com/Dubjay18/seraph/shared/money"
)

// ─── Stub repository ─────────────────────────────────────────────────────────

type stubRepo struct {
	idempotencyUsed bool
	posted          *service.PostRequest
	balance         money.Money
}

func (s *stubRepo) PostTransactionTx(_ context.Context, req service.PostRequest) (string, error) {
	s.posted = &req
	return "txn-001", nil
}

func (s *stubRepo) IsIdempotencyKeyUsed(_ context.Context, _ string) (bool, error) {
	return s.idempotencyUsed, nil
}

func (s *stubRepo) GetBalance(_ context.Context, _ string, _ money.Currency) (money.Money, error) {
	return s.balance, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func debit(accountID string, m money.Money) service.Entry {
	return service.Entry{AccountID: accountID, Type: service.EntryDebit, Amount: m}
}

func credit(accountID string, m money.Money) service.Entry {
	return service.Entry{AccountID: accountID, Type: service.EntryCredit, Amount: m}
}

// ─── PostTransaction ─────────────────────────────────────────────────────────

func TestPostTransaction_ValidNGN(t *testing.T) {
	repo := &stubRepo{}
	svc := service.New(repo)

	amount := money.MustNew(50000, money.NGN) // ₦500.00
	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "key-1",
		Description:    "test payment",
		Entries: []service.Entry{
			debit("sender-account", amount),
			credit("receiver-account", amount),
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.posted == nil {
		t.Fatal("expected PostTransactionTx to be called")
	}
}

func TestPostTransaction_ValidUSD(t *testing.T) {
	repo := &stubRepo{}
	svc := service.New(repo)

	amount := money.MustNew(1999, money.USD) // $19.99
	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "key-usd-1",
		Entries: []service.Entry{
			debit("acct-a", amount),
			credit("acct-b", amount),
		},
	})
	if err != nil {
		t.Fatalf("expected no error for USD, got: %v", err)
	}
}

func TestPostTransaction_ImbalancedRejected(t *testing.T) {
	repo := &stubRepo{}
	svc := service.New(repo)

	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "key-bad",
		Entries: []service.Entry{
			debit("a", money.MustNew(500, money.NGN)),
			credit("b", money.MustNew(400, money.NGN)), // 500 debit ≠ 400 credit
		},
	})
	if err == nil {
		t.Fatal("expected error for imbalanced entries")
	}
}

func TestPostTransaction_CrossCurrencyRejected(t *testing.T) {
	repo := &stubRepo{}
	svc := service.New(repo)

	// Debit NGN but credit USD — each currency is unbalanced.
	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "key-fx-bad",
		Entries: []service.Entry{
			debit("a", money.MustNew(500, money.NGN)),
			credit("b", money.MustNew(500, money.USD)),
		},
	})
	if err == nil {
		t.Fatal("expected error for cross-currency imbalance without FX entry")
	}
}

func TestPostTransaction_EmptyEntriesRejected(t *testing.T) {
	repo := &stubRepo{}
	svc := service.New(repo)

	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "key-empty",
	})
	if err == nil {
		t.Fatal("expected error for empty entries")
	}
}

func TestPostTransaction_IdempotentSkip(t *testing.T) {
	repo := &stubRepo{idempotencyUsed: true}
	svc := service.New(repo)

	amount := money.MustNew(100, money.GBP)
	_, err := svc.PostTransaction(context.Background(), service.PostRequest{
		IdempotencyKey: "duplicate-key",
		Entries: []service.Entry{
			debit("a", amount),
			credit("b", amount),
		},
	})
	if err != nil {
		t.Fatalf("expected no error for idempotent skip, got: %v", err)
	}
	if repo.posted != nil {
		t.Fatal("expected PostTransactionTx NOT to be called for duplicate key")
	}
}

// ─── GetBalance ───────────────────────────────────────────────────────────────

func TestGetBalance_ValidCurrency(t *testing.T) {
	want := money.MustNew(10000, money.NGN)
	repo := &stubRepo{balance: want}
	svc := service.New(repo)

	got, err := svc.GetBalance(context.Background(), "acct-x", money.NGN)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equals(want) {
		t.Errorf("GetBalance = %v, want %v", got, want)
	}
}

func TestGetBalance_InvalidCurrencyRejected(t *testing.T) {
	svc := service.New(&stubRepo{})
	_, err := svc.GetBalance(context.Background(), "acct-x", "ZZZ")
	if err == nil {
		t.Fatal("expected error for unsupported currency")
	}
}
