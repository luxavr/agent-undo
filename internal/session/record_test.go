package session

import "testing"

func TestAllowsMatrix(t *testing.T) {
	t.Parallel()
	sid := NewSessionID()
	cp := "cp_abc"
	fin := "cp_final"
	cases := []struct {
		name                     string
		rec                      Record
		show, diff, verify, undo bool
	}{
		{
			name: "created",
			rec:  Record{ID: sid, State: StateCreated},
			show: true,
		},
		{
			name: "checkpointing",
			rec:  Record{ID: sid, State: StateCheckpointing, CheckpointID: cp},
			show: true,
		},
		{
			name: "running",
			rec:  Record{ID: sid, State: StateRunning, CheckpointID: cp},
			show: true,
		},
		{
			name: "completed",
			rec:  Record{ID: sid, State: StateCompleted, CheckpointID: cp, FinalManifestID: fin, Process: ProcessInfo{Started: true}},
			show: true, diff: true, verify: true, undo: true,
		},
		{
			name: "interrupted",
			rec:  Record{ID: sid, State: StateInterrupted, CheckpointID: cp, FinalManifestID: fin, Process: ProcessInfo{Started: true}},
			show: true, diff: true, verify: true, undo: true,
		},
		{
			name: "failed before checkpoint",
			rec:  Record{ID: sid, State: StateFailed, Outcome: OutcomeCheckpointFailed},
			show: true,
		},
		{
			name: "failed after checkpoint no final",
			rec:  Record{ID: sid, State: StateFailed, Outcome: OutcomeFinalSnapshotFailed, CheckpointID: cp, Process: ProcessInfo{Started: true}},
			show: true, verify: true, undo: true,
		},
		{
			name: "failed spawn after checkpoint",
			rec:  Record{ID: sid, State: StateFailed, Outcome: OutcomeAgentUndoError, CheckpointID: cp},
			show: true, verify: true,
		},
		{
			name: "failed agent undo after start",
			rec:  Record{ID: sid, State: StateFailed, Outcome: OutcomeAgentUndoError, CheckpointID: cp, Process: ProcessInfo{Started: true}},
			show: true, verify: true, undo: true,
		},
		{
			name: "failed with both manifests",
			rec:  Record{ID: sid, State: StateFailed, CheckpointID: cp, FinalManifestID: fin, Process: ProcessInfo{Started: true}},
			show: true, diff: true, verify: true, undo: true,
		},
		{
			name: "v0.1.0 started infers exit code",
			rec:  Record{ID: sid, State: StateCompleted, CheckpointID: cp, FinalManifestID: fin, Process: ProcessInfo{ExitCode: intPtr(0)}},
			show: true, diff: true, verify: true, undo: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rec.Allows(OpShow) != tc.show {
				t.Fatalf("show: got %v want %v", tc.rec.Allows(OpShow), tc.show)
			}
			if tc.rec.Allows(OpDiff) != tc.diff {
				t.Fatalf("diff: got %v want %v", tc.rec.Allows(OpDiff), tc.diff)
			}
			if tc.rec.Allows(OpVerify) != tc.verify {
				t.Fatalf("verify: got %v want %v", tc.rec.Allows(OpVerify), tc.verify)
			}
			if tc.rec.Allows(OpUndo) != tc.undo {
				t.Fatalf("undo: got %v want %v", tc.rec.Allows(OpUndo), tc.undo)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
