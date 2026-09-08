package ignore

import (
	"io/fs"
	"testing"
)

func TestSkipDirs(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".git", "node_modules", "vendor", "dist", "build", "target", ".next", "__pycache__", ".venv", "venv"} {
		d := Dir(name)
		if !d.SkipDir || d.Class == "" {
			t.Fatalf("%s: %+v", name, d)
		}
	}
	if Dir("src").SkipDir {
		t.Fatal("src must not skip")
	}
	if Dir("env").SkipDir {
		t.Fatal("env/ is not a venv class")
	}
}

func TestTrackedUncapped(t *testing.T) {
	t.Parallel()
	d := File(0o644, MaxUntrackedBytes+1, GitTracked)
	if !d.Capture {
		t.Fatalf("%+v", d)
	}
}

func TestUntrackedOversize(t *testing.T) {
	t.Parallel()
	for _, g := range []GitClass{GitUnknown, GitUntracked, GitIgnored} {
		d := File(0o644, MaxUntrackedBytes+1, g)
		if d.Capture || d.Class != ClassOversize {
			t.Fatalf("git=%d %+v", g, d)
		}
	}
	d := File(0o644, MaxUntrackedBytes, GitIgnored)
	if !d.Capture {
		t.Fatal("1 MiB ignored file must be captured")
	}
}

func TestSymlinkAlwaysCapture(t *testing.T) {
	t.Parallel()
	d := File(fs.ModeSymlink|0o777, 99, GitUnknown)
	if !d.Capture {
		t.Fatal(d)
	}
}

func TestNonRegular(t *testing.T) {
	t.Parallel()
	if File(fs.ModeSocket, 0, GitUnknown).Class != ClassSocket {
		t.Fatal("socket")
	}
	if File(fs.ModeDevice, 0, GitUnknown).Class != ClassDevice {
		t.Fatal("device")
	}
	if File(fs.ModeNamedPipe, 0, GitUnknown).Class != ClassNonRegular {
		t.Fatal("fifo")
	}
}

func FuzzFile(f *testing.F) {
	f.Add(uint32(0o644), int64(100), 0)
	f.Add(uint32(fs.ModeSymlink), int64(0), 2)
	f.Add(uint32(0o644), int64(MaxUntrackedBytes+8), 1)
	f.Fuzz(func(t *testing.T, mode uint32, size int64, git int) {
		if size < 0 {
			size = 0
		}
		d := File(fs.FileMode(mode), size, GitClass(git%4))
		if d.Capture && d.SkipDir {
			t.Fatal("cannot capture and skip")
		}
		if !d.Capture && d.Class == "" && !d.SkipDir {
			t.Fatalf("reject without class: %+v", d)
		}
	})
}
