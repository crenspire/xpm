package deps

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		eco, a, b string
		want      int
	}{
		{"node", "1.2.3", "1.10.0", -1},
		{"node", "1.10.0", "1.2.3", 1},
		{"node", "1.2", "1.2.0", 0},
		{"node", "v1.2.3", "1.2.3", 0},
		{"node", "1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"node", "1.0.0-beta", "1.0.0", -1},
		{"node", "1.0.0", "1.0.0-beta", 1},
		{"node", "1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"node", "1.0.0-1", "1.0.0-alpha", -1},
		{"node", "1.0.0+build", "1.0.0", 0},
		{"node", "1.0.0+a", "1.0.0+b", 0},
		{"node", "2.0.0", "10.0.0", -1},
		{"python", "2.0.0rc1", "2.0.0", -1},
		{"python", "2.0rc2", "2.0rc10", -1},
		{"python", "1.0.dev1", "1.0", -1},
		{"python", "1.0a1", "1.0b1", -1},
		{"python", "1.0.post1", "1.0", 1},
		{"python", "1.0.0", "1.0", 0},
		{"java", "5.3.20", "6.0.0", -1},
		{"java", "1.0-SNAPSHOT", "1.0", -1},
		{"java", "1.0", "1.0-SNAPSHOT", 1},
		{"go", "v0.0.0-20230101000000-abcdef123456", "v0.1.0", -1},
		{"go", "v2.0.0+incompatible", "v2.0.0", 0},
		{"go", "v2.0.0+incompatible", "v1.9.9", 1},
		{"go", "1.2.3", "v1.2.3", 0},
		{"go", "v1.2.3", "v1.10.0", -1},
		{"go", "bogus", "v0.0.1", -1},
		{"go", "bogus", "alsobogus", 0},
		{"go", "v1.0.0-1", "v1.0.0-alpha", -1},
		{"go", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"java", "5.3.20.RELEASE", "5.3.20", 0},
		{"java", "5.3.20.Final", "5.3.20", 0},
		{"java", "5.4.0.Final", "5.4.0", 0},
		{"java", "1.0.0.GA", "1.0.0", 0},
		{"java", "1.0-Final", "1.0", 0},
		{"java", "6.0.0.Beta1", "6.0.0", -1},
		{"java", "6.0.0.M1", "6.0.0.RC1", -1},
		{"java", "6.0.0.RC1", "6.0.0", -1},
		{"java", "6.0.0.Alpha2", "6.0.0.Beta1", -1},
		{"java", "6.0.0.M2", "6.0.0.M10", -1},
		{"java", "6.0.0.RC1", "6.0.0.CR1", 0},
		{"java", "6.0.0.RC2", "6.0.0-SNAPSHOT", -1},
		{"java", "1.0.SP1", "1.0", 1},
		{"java", "1.0.foo", "1.0", 1},
		{"java", "1.0.bar", "1.0.foo", -1},
		{"python", "1.0alpha1", "1.0a1", 0},
		{"python", "1.0beta1", "1.0b1", 0},
		{"python", "1.0c1", "1.0rc1", 0},
		{"python", "1.0pre1", "1.0rc1", 0},
		{"python", "1.0preview1", "1.0rc1", 0},
		{"python", "1.0.dev1", "1.0a1", -1},
		{"python", "1.0a1", "1.0b1", -1},
		{"python", "1.0b1", "1.0rc1", -1},
		{"python", "1.0rc1", "1.0", -1},
		{"python", "1.0.post1", "1.0", 1},
		{"python", "1.0.post1", "1.0.post2", -1},
		{"node", "", "1.0.0", -1},
		{"node", "", "", 0},
		{"go", "", "v0.0.1", -1},
		{"node", "1.0-RC1", "1.0-rc1", 0},
	}
	for _, c := range cases {
		if got := Compare(c.eco, c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %q, %q) = %d, want %d", c.eco, c.a, c.b, got, c.want)
		}
		if got := Compare(c.eco, c.b, c.a); got != -c.want {
			t.Errorf("Compare(%s, %q, %q) = %d, want %d (antisymmetry)", c.eco, c.b, c.a, got, -c.want)
		}
	}
}
