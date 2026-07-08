package vision

import "testing"

func TestCoverTextScore(t *testing.T) {
	cases := []struct {
		desc    string
		wantPos bool // ожидаем положительный скор (годится в обложку)
	}{
		{"a small wooden cabin with two chairs on the porch", true},
		{"an a-frame house exterior in a snowy forest", true},
		{"snow-covered trees and a clear blue sky, a serene winter scene", false},
		{"a cozy interior with a bed and a sofa", false},
		{"a plate of food on a table", false},
		{"a person standing in front of a house", false}, // люди перевешивают
	}
	for _, c := range cases {
		if s := coverTextScore(c.desc); (s > 0) != c.wantPos {
			t.Errorf("desc %q: score %d, ожидал positive=%v", c.desc, s, c.wantPos)
		}
	}
}
