package domain

import "sort"

// SegmentPlan describes one segment on a track — either a real cut of
// the source ("clip") or a synthetic fill ("gap"). The planner expands
// a track of clips-plus-implicit-gaps into a flat slice that the filter
// builder can iterate without branching on gap/clip per segment.
type SegmentPlan struct {
	IsGap       bool
	SourceStart float64 // when !IsGap
	SourceEnd   float64 // when !IsGap
	Duration    float64 // when IsGap; for clips equals SourceEnd-SourceStart
}

// PlanSegments lays out one track as an alternating sequence of clip
// and gap segments. When totalDur > track length, a trailing gap is
// appended so the rendered stream matches the overall program duration —
// this is what keeps the muxed mp4's two streams equal-length when one
// track is edited shorter than the other.
//
// An empty track returns a nil plan; callers decide whether to emit a
// pure-gap stream (single-video doesn't; multitrack might).
func PlanSegments(clips []Clip, totalDur float64) []SegmentPlan {
	if len(clips) == 0 {
		return nil
	}
	sorted := append([]Clip(nil), clips...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ProgramStart < sorted[j].ProgramStart })

	var plan []SegmentPlan
	var cursor float64
	for _, c := range sorted {
		if c.ProgramStart > cursor+SnapEpsilon {
			plan = append(plan, SegmentPlan{IsGap: true, Duration: c.ProgramStart - cursor})
		}
		plan = append(plan, SegmentPlan{
			SourceStart: c.SourceStart,
			SourceEnd:   c.SourceEnd,
			Duration:    c.Duration(),
		})
		cursor = c.ProgramStart + c.Duration()
	}
	if totalDur > cursor+1e-3 {
		plan = append(plan, SegmentPlan{IsGap: true, Duration: totalDur - cursor})
	}
	return plan
}
