package regex

import (
	"regexp/syntax"
	"testing"
)

func TestReversedPattern(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{`x`, `x`},
		{`abc`, `cba`},
		{`[A-Z]+`, `[A-Z]+`},
		{`[\+\-]b[0-3]`, `[0-3]b[\+\-]`},
		{`ax?`, `x?a`},
		{`abc|123|z`, `cba|321|z`},
		{`x{2,3}a`, `ax{2,3}`},
		{`(abc)*`, `(cba)*`},
		{`(abc)+`, `(cba)+`},
		{`(abc){0,3}`, `(cba){0,3}`},
	}

	for _, test := range tests {
		re, err := syntax.Parse(test.expr, syntax.Perl)
		if err != nil {
			t.Fatalf("parse(%s): %v", test.expr, err)
		}
		have := reversedPattern(re)
		if have != test.want {
			t.Errorf("results mismatch for %s:\nhave: %s\nwant: %s",
				test.expr, have, test.want)
		}
	}
}
