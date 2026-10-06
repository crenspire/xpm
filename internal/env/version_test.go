package env

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in    string
		nums  []int
		pre   []string
		build []string
	}{
		{"1.22.3", []int{1, 22, 3}, nil, nil},
		{"v20.11.0", []int{20, 11, 0}, nil, nil},
		{"1.22", []int{1, 22}, nil, nil},
		{"1.22rc1", []int{1, 22}, []string{"rc", "1"}, nil},
		{"3.15.0rc3", []int{3, 15, 0}, []string{"rc", "3"}, nil},
		{"21.0.12.1+1", []int{21, 0, 12, 1}, nil, []string{"1"}},
		{"20.11.0-beta.1", []int{20, 11, 0}, []string{"beta", "1"}, nil},
		{"8u422-b05", []int{8}, []string{"u", "422", "b", "05"}, nil},
	}
	for _, c := range cases {
		v, ok := ParseVersion(c.in)
		if !ok {
			t.Errorf("ParseVersion(%q) failed", c.in)
			continue
		}
		if !reflect.DeepEqual(v.Nums, c.nums) || !reflect.DeepEqual(v.Pre, c.pre) || !reflect.DeepEqual(v.Build, c.build) {
			t.Errorf("ParseVersion(%q) = %+v", c.in, v)
		}
	}
	for _, bad := range []string{"", "lts", "latest", "stable", "1.", "1.2.3-", "1..2", "x1", "1.2.3+", "1.2.3_4", "1234567890"} {
		if _, ok := ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) succeeded, want failure", bad)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	less := [][2]string{
		{"1.9.0", "1.10.0"},
		{"1.22rc1", "1.22.0"},
		{"1.22rc2", "1.22rc10"},
		{"3.15.0rc3", "3.15.0"},
		{"20.11.0-beta.1", "20.11.0"},
		{"20.11.0-rc.2", "20.11.0-rc.10"},
		{"20.11.0-alpha", "20.11.0-beta"},
		{"21.0.12+1", "21.0.12.1+1"},
		{"21.0.12.1+1", "21.0.12.1+2"},
		{"8u412-b08", "8u422-b05"},
		{"lts", "1.0.0"},
	}
	for _, p := range less {
		if CompareVersions(p[0], p[1]) >= 0 || CompareVersions(p[1], p[0]) <= 0 {
			t.Errorf("want %s < %s", p[0], p[1])
		}
	}
	if CompareVersions("20.11.0", "v20.11.0") != 0 {
		t.Error("a leading v must not matter")
	}
}

func TestSortVersionsDesc(t *testing.T) {
	got := []string{"1.9.0", "1.10.0", "1.22rc1", "1.22.0", "1.2.0"}
	SortVersionsDesc(got)
	want := []string{"1.22.0", "1.22rc1", "1.10.0", "1.9.0", "1.2.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatchesSpec(t *testing.T) {
	yes := [][2]string{
		{"20", "20.11.0"}, {"20.11", "20.11.0"}, {"20.11.0", "20.11.0"},
		{"latest", "1.0.0"}, {"21", "21.0.12.1+1"}, {"8", "8u422-b05"},
		{"1.22rc1", "1.22rc1"},
	}
	no := [][2]string{
		{"20", "200.1.0"}, {"1.2", "1.20.3"}, {"20.11.0", "20.11"},
		{"1.22rc1", "1.22rc2"}, {"lts", "20.11.0"}, {"20", "lts"},
	}
	for _, p := range yes {
		if !MatchesSpec(p[0], p[1]) {
			t.Errorf("MatchesSpec(%q, %q) = false", p[0], p[1])
		}
	}
	for _, p := range no {
		if MatchesSpec(p[0], p[1]) {
			t.Errorf("MatchesSpec(%q, %q) = true", p[0], p[1])
		}
	}
}

func TestHighestMatch(t *testing.T) {
	versions := []string{"20.9.0", "20.11.1", "20.11.0", "200.1.0", "21.0.0-rc.1", "21.0.0-rc.2", "18.19.0"}
	cases := []struct {
		spec string
		pre  bool
		want string
		ok   bool
	}{
		{"20", false, "20.11.1", true},
		{"20.9", false, "20.9.0", true},
		{"latest", false, "200.1.0", true},
		{"21", false, "", false},
		{"21", true, "21.0.0-rc.2", true},
		{"19", false, "", false},
	}
	for _, c := range cases {
		got, ok := HighestMatch(c.spec, versions, c.pre)
		if got != c.want || ok != c.ok {
			t.Errorf("HighestMatch(%q, pre=%v) = %q, %v; want %q, %v", c.spec, c.pre, got, ok, c.want, c.ok)
		}
	}
}
