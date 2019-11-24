package regex

import (
	"regexp/syntax"
)

func reversedPattern(re *syntax.Regexp) string {
	// TODO(qusilyte): print regexp by hand, since String()
	// method sometimes returns weird patterns.
	// Also return <string, bool> with bool=false if
	// re contains begin/end line or begin/end text.
	//
	reversed := reversedRegexp(re)
	return reversed.String()
}

func reversedRegexp(re *syntax.Regexp) *syntax.Regexp {
	out := *re

	reversedSub := func(sub []*syntax.Regexp) []*syntax.Regexp {
		rsub := make([]*syntax.Regexp, 0, len(sub))
		for i := len(sub) - 1; i >= 0; i-- {
			rsub = append(rsub, reversedRegexp(sub[i]))
		}
		return rsub
	}

	switch re.Op {
	case syntax.OpAlternate:
		out.Sub = make([]*syntax.Regexp, len(re.Sub))
		for i, sub := range re.Sub {
			out.Sub[i] = reversedRegexp(sub)
		}
	case syntax.OpConcat:
		out.Sub = reversedSub(re.Sub)
	case syntax.OpCapture, syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		out.Sub[0] = reversedRegexp(re.Sub[0])
	case syntax.OpLiteral:
		out.Rune = make([]rune, 0, len(re.Rune))
		for i := len(re.Rune) - 1; i >= 0; i-- {
			out.Rune = append(out.Rune, re.Rune[i])
		}
	}

	return &out
}
