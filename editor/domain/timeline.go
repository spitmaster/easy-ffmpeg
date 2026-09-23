package domain

import commondomain "easy-ffmpeg/editor/common/domain"

// Timeline ops are pure functions over []Clip. The implementations live
// in editor/common/domain — re-exported here as function variables so
// existing single-video call sites (handlers, tests) continue to use the
// editor/domain.Foo names without churn. Multitrack imports the common
// package directly.

// ErrClipNotFound is returned when an operation targets a clip id that
// is not present in the timeline, or a program time falls in a gap.
var ErrClipNotFound = commondomain.ErrClipNotFound

// Function variables forward to the shared implementations. Using `var
// X = pkg.X` (vs. wrapper funcs) keeps the call surface lean — there's
// no new function body to test, the shared package's tests already cover
// behavior.
var (
	Split             = commondomain.Split
	DeleteClip        = commondomain.DeleteClip
	Reorder           = commondomain.Reorder
	TrimLeft          = commondomain.TrimLeft
	TrimRight         = commondomain.TrimRight
	SetProgramStart   = commondomain.SetProgramStart
	ClipAtProgramTime = commondomain.ClipAtProgramTime
)
