package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestConcurrentAdminDemotionsNeverRemoveEveryAdmin(t *testing.T) {
	ctx := context.Background()
	s, err := New("file:last-admin?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Exercise the race repeatedly: both calls can currently observe the other
	// administrator before either write commits. A correct repository serializes
	// the guard check and mutation, so every round leaves exactly one admin.
	for round := 0; round < 40; round++ {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM users`); err != nil {
			t.Fatalf("round %d reset users: %v", round, err)
		}
		for i := 0; i < 2; i++ {
			u := &domain.User{
				ID: fmt.Sprintf("admin-%d-%d", round, i), Username: fmt.Sprintf("admin-%d-%d", round, i),
				Password: "x", Role: domain.RoleAdmin, CreatedAt: time.Now().UTC(),
			}
			if err := s.Users().Create(ctx, u); err != nil {
				t.Fatalf("round %d create admin: %v", round, err)
			}
		}

		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			id := fmt.Sprintf("admin-%d-%d", round, i)
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- s.Users().UpdateRole(ctx, id, domain.RoleEditor)
			}()
		}
		close(start)
		wg.Wait()
		close(errs)

		var successes, guarded int
		for err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, storepkg.ErrLastAdmin):
				guarded++
			default:
				t.Fatalf("round %d unexpected demotion error: %v", round, err)
			}
		}
		var admins int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			t.Fatalf("round %d count admins: %v", round, err)
		}
		if admins != 1 || successes != 1 || guarded != 1 {
			t.Fatalf("round %d: admins=%d successes=%d last-admin-errors=%d; want 1/1/1", round, admins, successes, guarded)
		}
	}
}
