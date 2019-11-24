package regex

import (
	"testing"
)

func TestReverseReader(t *testing.T) {
	tests := []struct {
		s    string
		want string
	}{
		{"", ""},
		{"a", "a"},
		{"λ", "λ"},
		{"abc", "cba"},
		{"狐b犬c", "c犬b狐"},
		{"😈imp", "pmi😈"},
		{"←→↑↓", "↓↑→←"},
	}

	for _, test := range tests {
		r := newReverseReader(test.s)
		for _, ch1 := range test.want {
			ch2, _, _ := r.ReadRune()
			if ch1 != ch2 {
				t.Fatalf("test(%q) failed: want %c, got %c", test.s, ch1, ch2)
			}
		}
	}
}
