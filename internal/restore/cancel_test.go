package restore

import (
	"context"
	"testing"
)

func TestCancelAtEachPoint(t *testing.T) {
	t.Parallel()
	points := []string{
		PointBeforeLock,
		PointAfterLock,
		PointAfterLoad,
		PointAfterValidation,
		PointDuringPlan,
		PointBeforeRecovery,
		PointAfterRecovery,
		PointBeforeHEADMove,
		PointAfterHEADMove,
		PointBeforeOverlay,
		PointDuringOverlay,
		PointBeforeVerify,
		PointDuringVerify,
	}
	needRecovery := map[string]bool{
		PointAfterRecovery:  true,
		PointBeforeHEADMove: true,
		PointAfterHEADMove:  true,
		PointBeforeOverlay:  true,
		PointDuringOverlay:  true,
		PointBeforeVerify:   true,
		PointDuringVerify:   true,
	}
	for _, point := range points {
		p := point
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			mustWrite(t, root+"/a", "1")
			store, b := harness(t, root)
			cp := mustCreate(t, store, b)
			mustWrite(t, root+"/a", "2")
			ctx, cancel := context.WithCancel(context.Background())
			rep := Run(ctx, Options{
				Boundary: b, Store: store, TargetID: cp.ID, Yes: true,
				After: func(got string) {
					if got == p {
						cancel()
					}
				},
			})
			if rep.Outcome == OutcomeSuccess {
				t.Fatalf("%s: cancellation must not be success", p)
			}
			if needRecovery[p] && rep.RecoveryID == "" {
				t.Fatalf("%s: recovery checkpoint required before mutation", p)
			}
			if p == PointBeforeLock && rep.RecoveryID != "" {
				t.Fatal("no recovery before recovery stage")
			}
		})
	}
}
