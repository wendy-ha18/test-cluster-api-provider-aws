package main

import (
	"reflect"
	"testing"
)

func TestFilterStableTags(t *testing.T) {
	input := []string{
		"v1.32.3",
		"v1.32.0",
		"v1.31.7",
		"v1.30.11",
		"v1.33.0-alpha.1",
		"v1.33.0-beta.0",
		"v1.32.0-rc.1",
		"v1.29.0",
		"not-a-version",
	}

	got := filterStableTags(input)

	want := []string{
		"v1.32.3",
		"v1.32.0",
		"v1.31.7",
		"v1.30.11",
		"v1.29.0",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterStableTags() = %v, want %v", got, want)
	}
}

func TestFilterStableTags_NoPreReleases(t *testing.T) {
	preReleases := []string{
		"v1.33.0-alpha.1",
		"v1.33.0-beta.0",
		"v1.33.0-rc.1",
	}

	got := filterStableTags(preReleases)
	if len(got) != 0 {
		t.Errorf("expected no stable tags from pre-releases, got %v", got)
	}
}

func TestGroupByMinor(t *testing.T) {
	tags := []string{
		"v1.32.3", "v1.32.0",
		"v1.31.7",
		"v1.30.11",
	}

	got := groupByMinor(tags)

	if len(got["1.32"]) != 2 {
		t.Errorf("1.32 should have 2 patches, got %d", len(got["1.32"]))
	}
	if len(got["1.31"]) != 1 {
		t.Errorf("1.31 should have 1 patch, got %d", len(got["1.31"]))
	}
	if len(got["1.30"]) != 1 {
		t.Errorf("1.30 should have 1 patch, got %d", len(got["1.30"]))
	}
}

func TestTopMinors(t *testing.T) {
	groups := map[string][]string{
		"1.29": {"1.29.5"},
		"1.30": {"1.30.11"},
		"1.31": {"1.31.7"},
		"1.32": {"1.32.3", "1.32.0"},
	}

	got := topMinors(groups, 3)
	want := []string{"1.32", "1.31", "1.30"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("topMinors() = %v, want %v", got, want)
	}
}

func TestTopMinors_FewerThanN(t *testing.T) {
	groups := map[string][]string{
		"1.32": {"1.32.0"},
	}

	got := topMinors(groups, 3)
	if len(got) != 1 {
		t.Errorf("expected 1 minor when only 1 available, got %d", len(got))
	}
}

func TestSortPatchesDesc(t *testing.T) {
	patches := []string{"1.32.0", "1.32.3", "1.32.1", "1.32.2"}
	sortPatchesDesc(patches)

	want := []string{"1.32.3", "1.32.2", "1.32.1", "1.32.0"}
	if !reflect.DeepEqual(patches, want) {
		t.Errorf("sortPatchesDesc() = %v, want %v", patches, want)
	}
}

func TestMinorGreater(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.32", "1.31", true},
		{"1.31", "1.32", false},
		{"1.32", "1.32", false},
		{"2.0", "1.99", true},
	}

	for _, tc := range cases {
		got := minorGreater(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("minorGreater(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPatchGreater(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.32.3", "1.32.2", true},
		{"1.32.2", "1.32.3", false},
		{"1.32.3", "1.32.3", false},
		{"1.32.10", "1.32.9", true},
	}

	for _, tc := range cases {
		got := patchGreater(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("patchGreater(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
