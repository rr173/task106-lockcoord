# Bug19 Fix: Atomic holder + lease transfer during lock migration

## Root cause

`internal/handover/manager.go:771` `executeLockTransfer` updates the lock holder and
the lease holder in **two separate, non-transactional SQL statements**:

```go
if err := m.storage.TransferLockHolder(item.ResourceKey, h.ToCaller, now); err != nil {  // step 1
    return err
}
...
if err := m.storage.TransferLeaseHolder(item.ResourceKey, h.ToCaller, newExpires, now); err != nil {  // step 2
    return err
}
ctx.appliedLocks = append(ctx.appliedLocks, item.ResourceKey)  // only reached on full success
```

- Step 1 commits the new holder immediately.
- If step 2 fails (e.g. a `BEFORE UPDATE` trigger on `leases` that `RAISE(FAIL)`s),
  step 1 is **already committed**. The function returns the error and does *not* push
  the lock into `ctx.appliedLocks`, so the per-handover `rollbackAllLocked` never tries
  to undo it. Result: lock holder = "to", lease holder = "from" — a half-completed
  migration. This is exactly what `TestLockTransferRollsBackWhenLeaseTransferFails`
  asserts must not happen (`lock.Holder` must stay `"from"`).

## Fix

Make the two updates atomic by wrapping them in a single SQLite transaction at the
storage layer. This matches the existing transaction pattern already in
`internal/storage/sqlite.go` (`Dequeue` at line 1194, `RechargeBudget` at line 5353:
`tx, err := s.db.Begin(); defer tx.Rollback(); … tx.Commit()`).

### Change 1 — `internal/storage/sqlite.go`

Add one new method next to the existing `TransferLockHolder`/`TransferLeaseHolder`
(line ~3746):

```go
// TransferLockAndLease atomically moves the lock holder and the active lease
// holder to newHolder. If the lease update fails (e.g. a trigger), the whole
// transaction is rolled back so the lock holder is left unchanged — no
// half-completed migration.
func (s *Storage) TransferLockAndLease(lockName, newHolder string, newLeaseExpiresAt time.Time, updatedAt time.Time) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.Exec(`UPDATE locks SET holder = ?, updated_at = ? WHERE name = ?`, newHolder, updatedAt, lockName); err != nil {
        return err
    }
    if _, err := tx.Exec(`UPDATE leases SET holder = ?, expires_at = ?, active = 1 WHERE lock_name = ? AND active = 1`, newHolder, newLeaseExpiresAt, lockName); err != nil {
        return err
    }

    return tx.Commit()
}
```

Leave `TransferLockHolder` and `TransferLeaseHolder` in place — `rollbackAllLocked`
(line 891-897) still uses them to reverse successful prior transfers, and removing
them would touch behavior beyond this bug. No new imports needed (`database/sql`
already imported).

### Change 2 — `internal/handover/manager.go` (`executeLockTransfer`, line 771)

Replace the two separate storage calls with the single atomic call:

```go
func (m *Manager) executeLockTransfer(ctx *execContext, item *model.HandoverResourceItem, h *model.Handover, now time.Time) error {
    l, err := m.storage.GetLock(item.ResourceKey)
    if err != nil {
        return err
    }
    if l == nil || l.Holder != h.FromCaller {
        return fmt.Errorf("lock no longer held by source")
    }

    // Look up remaining lease first so the new holder keeps the same expiry,
    // and so we know whether to migrate a lease at all.
    newExpires := now
    lease, _ := m.storage.GetActiveLease(item.ResourceKey)
    if lease != nil {
        remaining := time.Until(lease.ExpiresAt)
        if remaining < 0 {
            remaining = 0
        }
        newExpires = now.Add(remaining)
    }

    // Atomically move both the lock holder and the lease holder. If the lease
    // transfer fails, the transaction rolls back and the lock holder stays with
    // the source — no half-completed migration.
    if err := m.storage.TransferLockAndLease(item.ResourceKey, h.ToCaller, newExpires, now); err != nil {
        return err
    }

    ctx.appliedLocks = append(ctx.appliedLocks, item.ResourceKey)
    return nil
}
```

Net effect: if `TransferLockAndLease` returns an error, neither the lock holder nor
the lease holder has changed, so there is nothing for `rollbackAllLocked` to undo for
this lock and no partial state is left behind.

## Why not the simpler "rollback inside executeLockTransfer" approach

A compensating `TransferLockHolder(item.ResourceKey, h.FromCaller, now)` on failure
would also satisfy the test, but it is weaker: between the commit of step 1 and the
compensating update, the lock is observably held by `to` with a lease still owned by
`from`, and a crash there leaves the half-migration on disk. The transactional fix
makes the two updates atomic at the DB level — the never-observable, crash-safe
property the bug report asks for ("原子更新").

## Constraints honored

- No test / config / dependency changes: only `internal/storage/sqlite.go` and
  `internal/handover/manager.go` production code. `bug19_test.go` untouched.
- `execContext` and `executeLockTransfer` stay unexported and callable from the test.
- `go vet`, `go build`, and the full `go test ./...` suite must pass.

## Verification (before & after, identical commands)

Pre-fix baseline (already captured): `go test ./...` exits 1 with only
`TestLockTransferRollsBackWhenLeaseTransferFails` failing; `go vet ./...` and
`go build ./...` exit 0.

Post-fix: run the exact same commands and confirm the test now passes and nothing
else regresses, with full output and real exit codes retained (no `|| true`, no
truncation).
