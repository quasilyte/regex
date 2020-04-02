package syntax

import (
	"fmt"
	"regexp/syntax"
	"strings"
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

	p := NewParser(nil)
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

func writeExpr(t *testing.T, w *strings.Builder, re *Regexp, e Expr) {
	assertBeginPos := func(e Expr, begin uint16) {
		if e.Begin() != begin {
			t.Errorf("`%s`: %s begin pos mismatch:\nhave: `%s` (begin=%d)\nwant: `%s` (begin=%d)",
				re.Source, e.Op,
				re.Source[e.Begin():e.End()], e.Begin(),
				re.Source[begin:e.End()], begin)
		}
	}
	assertEndPos := func(e Expr, end uint16) {
		if e.End() != end {
			t.Errorf("`%s`: %s end pos mismatch:\nhave: `%s` (end=%d)\nwant: `%s` (end=%d)",
				re.Source, e.Op,
				re.Source[e.Begin():e.End()], e.End(),
				re.Source[e.Begin():end], end)
		}
	}

	switch e.Op {
	case OpChar, OpString, OpQuote, OpPosixClass,
		OpEscape, OpEscapeMeta, OpEscapeUni, OpEscapeUniFull,
		OpEscapeHex, OpEscapeHexFull, OpEscapeOctal,
		OpDot, OpCaret, OpDollar:
		w.WriteString(re.ExprString(e))

	case OpLiteral:
		assertBeginPos(e, e.Args[0].Begin())
		assertEndPos(e, e.LastArg().End())
		for _, a := range e.Args {
			writeExpr(t, w, re, a)
		}

	case OpCharRange:
		assertBeginPos(e, e.Args[0].Begin())
		assertEndPos(e, e.Args[1].End())
		writeExpr(t, w, re, e.Args[0])
		w.WriteByte('-')
		writeExpr(t, w, re, e.Args[1])

	case OpNamedCapture:
		assertEndPos(e, e.Args[0].End()+1)
		fmt.Fprintf(w, "(?P<%s>", re.ExprString(e.Args[1]))
		writeExpr(t, w, re, e.Args[0])
		w.WriteByte(')')

	case OpFlagOnlyGroup:
		assertEndPos(e, e.Args[0].End()+1)
		w.WriteByte('(')
		w.WriteString(re.ExprString(e.Args[0]))
		w.WriteByte(')')

	case OpGroupWithFlags:
		assertEndPos(e, e.Args[0].End()+1)
		w.WriteByte('(')
		w.WriteString(re.ExprString(e.Args[1]))
		w.WriteByte(':')
		writeExpr(t, w, re, e.Args[0])
		w.WriteByte(')')

	case OpCapture, OpGroup:
		assertEndPos(e, e.Args[0].End()+1)
		w.WriteByte('(')
		if e.Op == OpGroup {
			w.WriteString("?:")
		}
		writeExpr(t, w, re, e.Args[0])
		w.WriteByte(')')

	case OpCharClass, OpNegCharClass:
		assertEndPos(e, e.LastArg().End()+1)
		w.WriteByte('[')
		if e.Op == OpNegCharClass {
			w.WriteByte('^')
		}
		for _, a := range e.Args {
			writeExpr(t, w, re, a)
		}
		w.WriteByte(']')

	case OpRepeat:
		assertBeginPos(e, e.Args[0].Begin())
		assertEndPos(e, e.Args[1].End())
		writeExpr(t, w, re, e.Args[0])
		writeExpr(t, w, re, e.Args[1])

	case OpConcat:
		assertBeginPos(e, e.Begin())
		if len(e.Args) > 0 {
			assertEndPos(e, e.LastArg().End())
		}
		for _, a := range e.Args {
			writeExpr(t, w, re, a)
		}

	case OpAlt:
		assertBeginPos(e, e.Begin())
		assertEndPos(e, e.LastArg().End())
		for i, a := range e.Args {
			writeExpr(t, w, re, a)
			if i != len(e.Args)-1 {
				w.WriteByte('|')
			}
		}

	case OpNonGreedy, OpQuestion, OpPlus, OpStar:
		assertEndPos(e, e.Args[0].End()+1)
		writeExpr(t, w, re, e.Args[0])
		switch e.Op {
		case OpNonGreedy, OpQuestion:
			w.WriteByte('?')
		case OpPlus:
			w.WriteByte('+')
		case OpStar:
			w.WriteByte('*')
		}

	default:
		panic(fmt.Sprintf("unhandled %s", e.Op))
	}
}

func TestWriteExpr(t *testing.T) {
	// Tests that ensure that we can print the source regexp
	// using the parsed AST.
	// They also verify that AST node positions are correct.

	tests := []struct {
		pat string
		o1  Operation
		o2  Operation
	}{
		{pat: `$`, o1: OpDollar},
		{pat: `abc`, o1: OpLiteral},
		{pat: `x{0}`, o1: OpChar, o2: OpString},
		{pat: `a\x{BAD}`, o1: OpLiteral, o2: OpEscapeHexFull},
		{pat: `(✓x✓x)`, o1: OpLiteral, o2: OpCapture},
		{pat: `[x]`, o1: OpCharClass, o2: OpLiteral},
		{pat: `[A-Za-z0-9-]`, o1: OpCharClass, o2: OpCharRange},
		{pat: `x{1}yz`, o1: OpLiteral, o2: OpRepeat},
		{pat: `x{1,2}y*`, o1: OpRepeat, o2: OpStar},
		{pat: `x{11,30}y+`, o1: OpRepeat, o2: OpPlus},
		{pat: `x{1,}$`, o1: OpRepeat, o2: OpDollar},
		{pat: `\p{Cyrillic}\d`, o1: OpEscapeUniFull, o2: OpEscape},
		{pat: `x\p{Greek}y+?`, o1: OpEscapeUniFull, o2: OpNonGreedy},
		{pat: `x\p{L}+y`, o1: OpEscapeUniFull, o2: OpPlus},
		{pat: `^\pL`, o1: OpEscapeUni, o2: OpCaret},
		{pat: `^x\pLy`, o1: OpEscapeUni, o2: OpCaret},
		{pat: `\d?`, o1: OpEscape, o2: OpQuestion},
		{pat: `[\xC0-\xC6]`, o1: OpCharRange, o2: OpEscapeHex},
		{pat: `\01\xff`, o1: OpEscapeOctal, o2: OpEscapeHex},
		{pat: `\111x\Qabc`, o1: OpEscapeOctal, o2: OpQuote},
		{pat: `x\Qabc\E.(?:s:..)`, o1: OpQuote, o2: OpGroupWithFlags},
		{pat: `(?i:foo[[:^alpha:]])`, o1: OpGroupWithFlags, o2: OpPosixClass},
		{pat: `a[[:digit:]\]]`, o1: OpPosixClass, o2: OpEscapeMeta},
		{pat: `(?:fa*)`, o1: OpGroup, o2: OpStar},
		{pat: `(?:x)|(?:y)`, o1: OpGroup, o2: OpAlt},
		{pat: `(foo|ba?r)`, o1: OpAlt, o2: OpQuestion},
		{pat: `(?P<1>xy\x{F})`, o1: OpNamedCapture, o2: OpEscapeHexFull},
		{pat: `(?P<x>)[^12]+?`, o1: OpNamedCapture, o2: OpNegCharClass},
		{pat: `()\(`, o1: OpCapture, o2: OpEscapeMeta},
		{pat: `x{1,}?.?.`, o1: OpNonGreedy, o2: OpDot},
		{pat: `(?i)f.o`, o1: OpFlagOnlyGroup, o2: OpDot},
		{pat: `(?:(?i)[^a-z]o)`, o1: OpFlagOnlyGroup, o2: OpNegCharClass},
		{pat: `(?:(?P<foo>x))`, o1: OpString, o2: OpChar},

		{pat: `\s*\{weight=(\d+)\}\s*`},
		{pat: `[.?,!;:@#$%^&*()]+`},
		{pat: `--(?P<var_name>[\\w-]+?):\\s+?(?P<var_val>.+?);`},
		{pat: `^ *(#{1,6}) *([^\n]+?) *#* *(?:\n|$)`},
		{pat: `^4\d{12}(\d{3})?$`},
	}

	const minTests = 2
	toCover := make(map[Operation]int)
	for op := OpNone + 1; op < OpNone2; op++ {
		switch op {
		case OpConcat:
			continue
		}
		toCover[op] = minTests
	}

	exprToString := func(re *Regexp) (s string, err error) {
		var b strings.Builder
		writeExpr(t, &b, re, re.Expr)
		return b.String(), nil
	}

	p := NewParser(nil)
	for _, test := range tests {
		pattern := "_" + test.pat + "_"
		re, err := p.Parse(pattern)
		if err != nil {
			t.Fatalf("parse(%q): %v", test.pat, err)
		}
		have, err := exprToString(re)
		if err != nil {
			t.Fatalf("stringify(%q): %v", test.pat, err)
		}
		want := pattern
		if have != want {
			t.Fatalf("result mismatch:\nhave: `%s`\nwant: `%s`", have, want)
		}
		if test.o1 != 0 {
			toCover[test.o1]--
		}
		if test.o2 != 0 {
			toCover[test.o2]--
			if test.o2 == test.o1 {
				t.Fatalf("%s: o1==o2", test.pat)
			}
		}
	}

	for op, n := range toCover {
		if n > 0 {
			t.Errorf("not enough tests for %s: want %d, have %d",
				op, minTests, minTests-n)
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
		{`  `, `  `},
		{`x`, `x`},
		{`abc`, `abc`},
		{`□`, `□`},
		{`✓`, `✓`},
		{`✓✓`, `✓✓`},

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
		{`(?:a|b)`, `(group (or a b))`},

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
		{`(ab)+`, `(+ (capture ab))`},
		{`(.b)+`, `(+ (capture {. b}))`},
		{`x+y*z+`, `{(+ x) (* y) (+ z)}`},
		{`abc+`, `{ab (+ c)}`},

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
		{`[0-9-]`, `[0-9 -]`},
		{`[\da-z_A-Z]`, `[\d a-z _ A-Z]`},
		{`[\(-\)ab]`, `[\(-\) a b]`},
		{`[\]\]\d]a`, `{[\] \] \d] a}`},
		{`[[\[]a`, `{[[ \[] a}`},
		{`[a|b]`, `[a | b]`},
		{`[a+b]`, `[a + b]`},
		{`[a*b]`, `[a * b]`},
		{`[x{1}]`, `[x '{' 1 '}']`},

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
		{`.{a}`, `{. {a}}`},
		{`.{-1}`, `{. {-1}}`},

		// \Q...\E escape.
		{`\Qa.b\E+z`, `{(+ (q \Qa.b\E)) z}`},
		{`x\Q?\Ey`, `{x (q \Q?\E) y}`},
		{`x\Q\Ey`, `{x (q \Q\E) y}`},
		{`x\Q`, `{x (q \Q)}`},
		{`x\Qy`, `{x (q \Qy)}`},
		{`x\Qyz`, `{x (q \Qyz)}`},

		// Incomplete `x|` expressions are valid.
		// `|x` is not valid though.
		{`(docker-|)`, `(capture (or docker- {}))`},
		{`x|`, `(or x {})`},

		// More tests for char merging.
		{`xy+`, `{x (+ y)}`},
		{`.xy`, `{. xy}`},

		// Tests from the patterns found in various GitHub projects.
		{`Adm([^i]|$)`, `{Adm (capture (or [^i] $))}`},
		{`\.(com|com\.\w{2})$`, `{\. (capture (or {c o m} {c o m \. (repeat \w {2})})) $}`},
		{`(?i)a(?:x|y)b`, `{(flags ?i) a (group (or x y)) b}`},
	}

	p := NewParser(nil)
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
			p := NewParser(nil)
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
