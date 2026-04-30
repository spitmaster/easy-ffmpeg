package domain

import "testing"

func TestPlanSegments_Empty(t *testing.T) {
	if PlanSegments(nil, 0) != nil {
		t.Error("nil clips should yield nil plan")
	}
	if PlanSegments(nil, 10) != nil {
		t.Error("nil clips with totalDur should still yield nil")
	}
}

func TestPlanSegments_ContiguousNoTrailingPad(t *testing.T) {
	clips := []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "b", SourceStart: 10, SourceEnd: 15, ProgramStart: 5},
	}
	plan := PlanSegments(clips, 10)
	if len(plan) != 2 {
		t.Fatalf("want 2 segments, got %d: %v", len(plan), plan)
	}
	if plan[0].IsGap || plan[0].SourceStart != 0 || plan[0].SourceEnd != 5 {
		t.Errorf("seg[0] = %+v", plan[0])
	}
	if plan[1].IsGap || plan[1].SourceStart != 10 {
		t.Errorf("seg[1] = %+v", plan[1])
	}
}

func TestPlanSegments_GapBetweenClips(t *testing.T) {
	clips := []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "b", SourceStart: 10, SourceEnd: 15, ProgramStart: 10}, // 5s gap [5..10]
	}
	plan := PlanSegments(clips, 15)
	if len(plan) != 3 {
		t.Fatalf("want 3 segments (clip+gap+clip), got %d", len(plan))
	}
	if plan[1].IsGap != true || plan[1].Duration != 5 {
		t.Errorf("middle should be 5s gap, got %+v", plan[1])
	}
}

func TestPlanSegments_TrailingPad(t *testing.T) {
	clips := []Clip{{ID: "a", SourceStart: 0, SourceEnd: 5, ProgramStart: 0}}
	plan := PlanSegments(clips, 12) // 5s clip + 7s tail
	if len(plan) != 2 {
		t.Fatalf("want 2 (clip + trailing), got %d", len(plan))
	}
	if !plan[1].IsGap || plan[1].Duration != 7 {
		t.Errorf("trailing seg should be 7s gap, got %+v", plan[1])
	}
}

func TestPlanSegments_OutOfOrderClipsSorted(t *testing.T) {
	clips := []Clip{
		{ID: "later", SourceStart: 10, SourceEnd: 15, ProgramStart: 10},
		{ID: "earlier", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
	}
	plan := PlanSegments(clips, 15)
	// Expect: clip[earlier] @ src 0..5, gap 5..10, clip[later] @ src 10..15
	if plan[0].SourceStart != 0 || plan[0].IsGap {
		t.Errorf("first segment should be the earlier clip, got %+v", plan[0])
	}
	if !plan[1].IsGap {
		t.Errorf("second segment should be the gap, got %+v", plan[1])
	}
	if plan[2].SourceStart != 10 {
		t.Errorf("third segment should be the later clip, got %+v", plan[2])
	}
}
