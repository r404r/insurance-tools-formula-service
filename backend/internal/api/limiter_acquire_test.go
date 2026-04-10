package api

import (
	"context"
	"testing"
	"time"
)

// TestAcquireReleaseBasic verifies that Acquire and Release work for simple
// acquisitions and that the inflight counter is balanced.
func TestAcquireReleaseBasic(t *testing.T) {
	d := NewDynamicConcurrencyLimiter(2)
	ctx := context.Background()

	r1, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if d.inflight.Load() != 1 {
		t.Errorf("inflight after 1 acquire = %d, want 1", d.inflight.Load())
	}

	r2, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	if d.inflight.Load() != 2 {
		t.Errorf("inflight after 2 acquires = %d, want 2", d.inflight.Load())
	}

	// Third Acquire should block; verify via short timeout.
	ctx3, cancel3 := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel3()
	if _, err := d.Acquire(ctx3); err != context.DeadlineExceeded {
		t.Errorf("third Acquire should block until ctx deadline, got err=%v", err)
	}

	r1()
	if d.inflight.Load() != 1 {
		t.Errorf("inflight after first release = %d, want 1", d.inflight.Load())
	}

	// Now Acquire should succeed.
	r3, err := d.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}

	r2()
	r3()
	if d.inflight.Load() != 0 {
		t.Errorf("inflight after all releases = %d, want 0", d.inflight.Load())
	}
}

// TestSetLimitWakesWaiters verifies that a blocked Acquire wakes and re-reads
// the limit when SetLimit retires the generation.
func TestSetLimitWakesWaiters(t *testing.T) {
	d := NewDynamicConcurrencyLimiter(1)
	ctx := context.Background()

	// Fill the single slot.
	r1, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}

	// Start a second Acquire in a goroutine — it must block on the current
	// generation's full sem.
	acquired := make(chan struct{})
	var r2 func()
	go func() {
		r, err := d.Acquire(ctx)
		if err != nil {
			t.Errorf("Acquire 2: %v", err)
		}
		r2 = r
		close(acquired)
	}()

	// Confirm it is actually blocked.
	select {
	case <-acquired:
		t.Fatal("second Acquire should have blocked on full limiter")
	case <-time.After(30 * time.Millisecond):
	}

	// Raise the limit. The waiter should wake, re-read the new (2-slot) sem,
	// and successfully acquire from it without depending on r1() releasing.
	d.SetLimit(2)

	select {
	case <-acquired:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("second Acquire did not wake after SetLimit")
	}

	r1()
	r2()
}

// TestReleaseOnOldGenerationDoesNotAffectNew verifies that releasing an
// old-generation token does not drain the new generation's semaphore.
func TestReleaseOnOldGenerationDoesNotAffectNew(t *testing.T) {
	d := NewDynamicConcurrencyLimiter(2)
	ctx := context.Background()

	// Acquire two slots on the initial generation.
	rOld1, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire old 1: %v", err)
	}
	rOld2, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire old 2: %v", err)
	}

	// Swap to a new generation with a smaller limit.
	d.SetLimit(1)

	// New-generation Acquire should see an empty 1-slot sem and succeed
	// immediately, even though two old holders are still alive.
	rNew, err := d.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire new 1: %v", err)
	}

	// Releasing the first old holder drains the (detached) old sem, not the
	// new sem. So another new-gen Acquire should still block.
	rOld1()
	ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel2()
	if _, err := d.Acquire(ctx2); err != context.DeadlineExceeded {
		t.Errorf("new-gen Acquire should block while new sem is full, got err=%v", err)
	}

	// Clean up.
	rOld2()
	rNew()
}
