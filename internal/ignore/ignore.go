package ignore

import "io/fs"

// MaxUntrackedBytes is the capture cap for untracked and gitignored files.
const MaxUntrackedBytes int64 = 1 << 20

// GitClass is how git sees a path. Unknown means git was not captured.
type GitClass int

const (
	GitUnknown GitClass = iota
	GitTracked
	GitUntracked
	GitIgnored
)

const (
	ClassGitDir      = "git.dir"
	ClassNodeModules = "node_modules"
	ClassVendor      = "vendor"
	ClassDist        = "dist"
	ClassBuild       = "build"
	ClassTarget      = "target"
	ClassNext        = "next"
	ClassPycache     = "pycache"
	ClassVenv        = "venv"
	ClassOversize    = "oversize"
	ClassSocket      = "socket"
	ClassDevice      = "device"
	ClassNonRegular  = "nonregular"
	ClassExternal    = "symlink.external"
)

var skipDirs = map[string]string{
	".git":         ClassGitDir,
	"node_modules": ClassNodeModules,
	"vendor":       ClassVendor,
	"dist":         ClassDist,
	"build":        ClassBuild,
	"target":       ClassTarget,
	".next":        ClassNext,
	"__pycache__":  ClassPycache,
	".venv":        ClassVenv,
	"venv":         ClassVenv,
}

// Decision is the capture classifier result.
type Decision struct {
	Capture bool
	SkipDir bool
	Class   string
}

// Dir reports whether a directory name should be skipped entirely.
func Dir(name string) Decision {
	if class, ok := skipDirs[name]; ok {
		return Decision{SkipDir: true, Class: class}
	}
	return Decision{Capture: true}
}

// File classifies a non-directory path.
func File(mode fs.FileMode, size int64, git GitClass) Decision {
	if mode&fs.ModeSymlink != 0 {
		return Decision{Capture: true}
	}
	if mode&fs.ModeSocket != 0 {
		return Decision{Class: ClassSocket}
	}
	if mode&fs.ModeDevice != 0 || mode&fs.ModeCharDevice != 0 {
		return Decision{Class: ClassDevice}
	}
	if mode&fs.ModeNamedPipe != 0 || !mode.IsRegular() {
		return Decision{Class: ClassNonRegular}
	}
	if git != GitTracked && size > MaxUntrackedBytes {
		return Decision{Class: ClassOversize}
	}
	return Decision{Capture: true}
}
