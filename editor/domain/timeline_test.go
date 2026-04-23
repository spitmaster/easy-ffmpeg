package domain

import (
	"errors"
	"testing"
)

// equalClips compares two slices for readable diff in tests.
func equalClips(t *testing.T, got, want []Clip) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d\ngot=%v\nwant=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("clip[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSplit(t *testing.T) {
	// One clip covering [0..20]. Program time t=10 lands at sourceTime 10.
	in := []Clip{{ID: "c1", SourceStart: 0, SourceEnd: 20}}
	got, err := Split(in, 10, "c2")
	if err != nil {
		t.Fatalf("Split error: %v", err)
	}
	equalClips(t, got, []Clip{
		{ID: "c1", SourceStart: 0, SourceEnd: 10},
		{ID: "c2", SourceStart: 10, SourceEnd: 20},
	})
	// input unchanged
	if in[0].SourceEnd != 20 {
		t.Errorf("original clips were mutated")
	}
}

func TestSplitAcrossClips(t *testing.T) {
	// Two clips stacked: program time 15 is inside clip[1] at sourceTime 15.
	in := []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 10},
		{ID: "b", SourceStart: 10, SourceEnd: 30}, // program range [10..30]
	}
	got, err := Split(in, 15, "b2")
	if err != nil {
		t.Fatalf("Split error: %v", err)
	}
	equalClips(t, got, []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 10},
		{ID: "b", SourceStart: 10, SourceEnd: 15},
		{ID: "b2", SourceStart: 15, SourceEnd: 30},
	})
}

func TestSplitErrors(t *testing.T) {
	in := []Clip{{ID: "c1", SourceStart: 0, SourceEnd: 20}}
	cases := []struct {
		name  string
		t     float64
		newID string
		want  error
	}{
		{"on start boundary", 0, "c2", nil /* boundary */},
		{"on end boundary", 20, "c2", ErrClipNotFound /* out of range */},
		{"past end", 25, "c2", ErrClipNotFound},
		{"empty id", 5, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Split(in, c.t, c.newID)
			if err == nil {
				t.Errorf("expected error for %s, got nil", c.name)
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Errorf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestDeleteClip(t *testing.T) {
	in := []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 10},
		{ID: "b", SourceStart: 10, SourceEnd: 20},
		{ID: "c", SourceStart: 20, SourceEnd: 30},
	}
	got, err := DeleteClip(in, "b")
	if err != nil {
		t.Fatalf("DeleteClip: %v", err)
	}
	equalClips(t, got, []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 10},
		{ID: "c", SourceStart: 20, SourceEnd: 30},
	})
	// unchanged input
	if len(in) != 3 {
		t.Error("input slice mutated")
	}

	if _, err := DeleteClip(in, "nope"); !errors.Is(err, ErrClipNotFound) {
		t.Errorf("want ErrClipNotFound, got %v", err)
	}
}

func TestReorder(t *testing.T) {
	in := []Clip{
		{ID: "a", SourceStart: 0, SourceEnd: 1},
		{ID: "b", SourceStart: 0, SourceEnd: 1},
		{ID: "c", SourceStart: 0, SourceEnd: 1},
		{ID: "d", SourceStart: 0, SourceEnd: 1},
	}
	got, err := Reorder(in, 0, 2)
	if err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	gotIDs := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID}
	want := []string{"b", "c", "a", "d"}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("order[%d]: got %q, want %q", i, gotIDs[i], want[i])
		}
	}

	// noop
	got, err = Reorder(in, 1, 1)
	if err != nil || got[1].ID != "b" {
		t.Errorf("self-reorder should be noop, got err=%v, ids[1]=%q", err, got[1].ID)
	}

	// out of range
	if _, err := Reorder(in, -1, 0); err == nil {
		t.Error("expected err for negative from")
	}
	if _, err := Reorder(in, 0, 99); err == nil {
		t.Error("expected err for out-of-range to")
	}
}

func TestTrimLeft(t *testing.T) {
	in := []Clip{{ID: "a", SourceStart: 5, SourceEnd: 20}}
	got, err := TrimLeft(in, "a", 8)
	if err != nil {
		t.Fatalf("TrimLeft: %v", err)
	}
	if got[0].SourceStart != 8 || got[0].SourceEnd != 20 {
		t.Errorf("got %v", got[0])
	}
	if in[0].SourceStart != 5 {
		t.Error("input mutated")
	}
	// validation errors
	if _, err := TrimLeft(in, "a", -1); err == nil {
		t.Error("negative start should error")
	}
	if _, err := TrimLeft(in, "a", 25); err == nil {
		t.Error("start >= end should error")
	}
	if _, err := TrimLeft(in, "nope", 10); !errors.Is(err, ErrClipNotFound) {
		t.Errorf("want ErrClipNotFound, got %v", err)
	}
}

func TestTrimRight(t *testing.T) {
	in := []Clip{{ID: "a", SourceStart: 5, SourceEnd: 20}}
	got, err := TrimRight(in, "a", 15)
	if err != nil {
		t.Fatalf("TrimRight: %v", err)
	}
	if got[0].SourceEnd != 15 {
		t.Errorf("got end %v", got[0].SourceEnd)
	}
	// end <= start
	if _, err := TrimRight(in, "a", 5); err == nil {
		t.Error("end = start should error")
	}
}
