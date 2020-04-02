package syntax

import (
	"regexp/syntax"
	"testing"
)

func TestParserErrors(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{`\`, `unexpected end of pattern: trailing '\'`},
		{`\x`, `unexpected end of pattern: expected hex-digit or '{'`},
		{`\x{12`, `can't find closing '}'`},
		{`(abc`, `expected ')', found 'None'`},
		{`[abc`, `unterminated '['`},
		{`\p`, `unexpected end of pattern: expected uni-class-short or '{'`},
		{`\p{L`, `can't find closing '}'`},
	}

	// TODO: handle incomplete expressions like `x|` and add error tests here.

	p := NewParser()
	for _, test := range tests {
		_, err := p.Parse(test.pattern)
		have := "<nil>"
		if err != nil {
			have = err.Error()
		}
		if have != test.want {
			t.Errorf("parse(%q):\nhave: %s\nwant: %s",
				test.pattern, have, test.want)
		}
	}
}

func TestParser(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		// Empty pattern.
		{``, `{}`},

		// Anchors.
		{`^`, `^`},
		{`^^`, `{^ ^}`},
		{`$`, `$`},
		{`$$`, `{$ $}`},

		// Simple literals and chars.
		{` `, ` `},
		{`  `, `{   }`},
		{`x`, `x`},
		{`abc`, `{a b c}`},
		{`□`, `□`},
		{`✓`, `✓`},
		{`✓✓`, `{✓ ✓}`},

		// Dots and alternations (or).
		{`.`, `.`},
		{`..`, `{. .}`},
		{`...`, `{. . .}`},
		{`.|.`, `(or . .)`},
		{`.|✓|.`, `(or . ✓ .)`},
		{`✓.|.`, `(or {✓ .} .)`},
		{`.|✓.`, `(or . {✓ .})`},
		{`..✓|.`, `(or {. . ✓} .)`},
		{`.|..|..✓`, `(or . {. .} {. . ✓})`},
		{`.|...|..`, `(or . {. . .} {. .})`},

		// Capturing groups.
		{`()`, `(capture {})`},
		{`(.)`, `(capture .)`},
		{`(.✓)`, `(capture {. ✓})`},
		{`(x)|(y)`, `(or (capture x) (capture y))`},
		{`(x)(y)`, `{(capture x) (capture y)}`},
		{`✓(x)y`, `{✓ (capture x) y}`},
		{`a(x1|y1)b`, `{a (capture (or {x 1} {y 1})) b}`},

		// Non-capturing groups without flags.
		{`x(?:)y`, `{x (group {}) y}`},
		{`x(?:.)y`, `{x (group .) y}`},
		{`x(?:ab)y`, `{x (group {a b}) y}`},

		// Flag-only groups.
		{`x(?i)y`, `{x (flags ?i) y}`},
		{`x(?i-m)y`, `{x (flags ?i-m) y}`},
		{`x(?-im)y`, `{x (flags ?-im) y}`},

		// Non-capturing groups with flags.
		{`x(?i:)y`, `{x (group {} ?i) y}`},
		{`x(?im:.)y`, `{x (group . ?im) y}`},
		{`x(?i-m:ab)y`, `{x (group {a b} ?i-m) y}`},

		// Named captures.
		{`x(?P<g>)y`, `{x (capture {} g) y}`},
		{`x(?P<name>.)y`, `{x (capture . name) y}`},
		{`x(?P<1>ab)y`, `{x (capture {a b} 1) y}`},

		// Quantifiers.
		{`x+`, `(+ x)`},
		{`x+|y+`, `(or (+ x) (+ y))`},
		{`x+y+`, `{(+ x) (+ y)}`},
		{`x+y+|z+`, `(or {(+ x) (+ y)} (+ z))`},
		{`(ab)+`, `(+ (capture {a b}))`},
		{`(.b)+`, `(+ (capture {. b}))`},
		{`x+y*z+`, `{(+ x) (* y) (+ z)}`},
		{`abc+`, `{a b (+ c)}`},

		// Non-greedy modifiers.
		{`x+?|y+?`, `(or (non-greedy (+ x)) (non-greedy (+ y)))`},
		{`x*?|y*?`, `(or (non-greedy (* x)) (non-greedy (* y)))`},
		{`x??|y??`, `(or (non-greedy (? x)) (non-greedy (? y)))`},

		// Escapes and escape chars.
		{`\d\d+`, `{\d (+ \d)}`},
		{`\..`, `{\. .}`},

		// Short Unicode escapes.
		{`\pL+d`, `{(+ \pL) d}`},

		// Full Unicode escapes.
		{`\p{Greek}\p{L}`, `{\p{Greek} \p{L}}`},
		{`\P{Greek}\p{^L}`, `{\P{Greek} \p{^L}}`},

		// Octal escapes.
		{`\0`, `\0`},
		{`\01`, `\01`},
		{`\012`, `\012`},
		{`\777`, `\777`},
		{`\78`, `{\7 8}`},
		{`\778`, `{\77 8}`},

		// Short hex escapes.
		{`\xfff`, `{\xff f}`},
		{`\xab1`, `{\xab 1}`},

		// Full hex escapes.
		{`\x{}b`, `{\x{} b}`},
		{`\x{1}b`, `{\x{1} b}`},
		{`\x{ABC}b`, `{\x{ABC} b}`},

		// Char classes.
		{`[]`, `[]`},
		{`[1]`, `[1]`},
		{`[]a`, `{[] a}`},
		{`[1]a`, `{[1] a}`},
		{`[-a]`, `[- a]`},
		{`[a-]`, `[a -]`},
		{`[a-z]a`, `{[a-z] a}`},
		{`[a-z0-9]`, `[a-z 0-9]`},
		{`[\da-z_A-Z]`, `[\d a-z _ A-Z]`},
		{`[\(-\)ab]`, `[\(-\) a b]`},
		{`[\]\]\d]a`, `{[\] \] \d] a}`},
		{`[[\[]a`, `{[[ \[] a}`},

		// Negated char classes.
		{`[^]`, `[^]`},
		{`[^1]a`, `{[^1] a}`},
		{`[^-a]`, `[^- a]`},
		{`[^a-]`, `[^a -]`},
		{`[^a-z]a`, `{[^a-z] a}`},
		{`[^a-z0-9]`, `[^a-z 0-9]`},
		{`[^\da-z_A-Z]`, `[^\d a-z _ A-Z]`},
		{`[^\(-\)ab]`, `[^\(-\) a b]`},
		{`[^\]\]\d]a`, `{[^\] \] \d] a}`},
		{`[^[\[]a`, `{[^[ \[] a}`},

		// Char class ranges.
		{`[\d-a]`, `[\d - a]`},
		{`[a-\d]`, `[a - \d]`},
		{`[\pL0-9]`, `[\pL 0-9]`},
		{`[+--]`, `[+--]`},
		{`[--+]`, `[--+]`},
		{`[---]`, `[---]`},
		{`[-]`, `[-]`},

		// Char class with meta symbols.
		{`[|]`, `[|]`},
		{`[$.+*^?]`, `[$ . + * ^ ?]`},
		{`[^$.+*^?]`, `[^$ . + * ^ ?]`},

		// Posix char classes.
		{`x[:alpha:]y`, `{x [: a l p h a :] y}`},
		{`x[a[:alpha:]]y`, `{x [a [:alpha:]] y}`},
		{`x[[:^alpha:]]y`, `{x [[:^alpha:]] y}`},
		{`x[^[:alpha:]]y`, `{x [^[:alpha:]] y}`},
		{`x[^[:^alpha:]]y`, `{x [^[:^alpha:]] y}`},

		// Valid repeat expressions.
		{`.{3}`, `(repeat . {3})`},
		{`.{3,}`, `(repeat . {3,})`},
		{`.{3,6}`, `(repeat . {3,6})`},
		{`.{6}?`, `(non-greedy (repeat . {6}))`},
		{`[a-z]{5}`, `(repeat [a-z] {5})`},

		// Invalid repeat expressions are parsed as normal chars.
		{`.{a}`, `{. '{' a '}'}`},
		{`.{-1}`, `{. '{' - 1 '}'}`},

		// \Q...\E escape.
		{`\Qa.b\E+z`, `{(+ (q \Qa.b\E)) z}`},
		{`x\Q?\Ey`, `{x (q \Q?\E) y}`},
		{`x\Q\Ey`, `{x (q \Q\E) y}`},
		{`x\Q`, `{x (q \Q)}`},
		{`x\Qy`, `{x (q \Qy)}`},
		{`x\Qyz`, `{x (q \Qyz)}`},

		// Tests from the patterns found in various GitHub projects.
		{`Adm([^i]|$)`, `{A d m (capture (or [^i] $))}`},
		{`\.(com|com\.\w{2})$`, `{\. (capture (or {c o m} {c o m \. (repeat \w {2})})) $}`},
	}

	p := NewParser()
	for _, test := range tests {
		re, err := p.Parse(test.pattern)
		if err != nil {
			t.Fatalf("parse(%q) error: %v", test.pattern, err)
		}
		have := FormatSyntax(re)
		if have != test.want {
			t.Fatalf("parse(%q):\nhave: %s\nwant: %s",
				test.pattern, have, test.want)
		}
	}
}

// To run benchmarks:
//	$ go-benchrun ParserStdlib ParserPratt -count 5
var benchmarkTests = []*struct {
	name    string
	pattern string
}{
	{`lit`, `\+\.1234foobarbaz✓✓□□`},
	{`alt`, `(x|y|1)|z|$`},
	{`esc`, `\w\d\pL\123\059\p{L}\p{^Greek}`},
	{`charclass`, `[a-z0-9_][^\d][\(-\)][1234][[[][a-][-a]`},
	{`posix`, `[[:alpha:][:blank:][:^word:]][[:^digit:]]`},
	{`meta`, `x+y*z?.*?.+?.??`},
	{`repeat`, `x{3,}\d{1,4}y{5}z{0}`},
	{`group`, `(?:x)(?i:(?i))(x)(?P<name>x)`},
	{`quote`, `\Qhttp://a.b.com/?x[]=1\E`},
}

func BenchmarkParserPratt(b *testing.B) {
	for _, test := range benchmarkTests {
		b.Run(test.name, func(b *testing.B) {
			p := NewParser()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.Parse(test.pattern)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkParserStdlib(b *testing.B) {
	for _, test := range benchmarkTests {
		b.Run(test.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := syntax.Parse(test.pattern, syntax.Perl)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
