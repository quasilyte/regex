package regex

import (
	"regexp"
	"regexp/syntax"
)

// Matcher reflects regexp match operations.
type Matcher interface {
	MatchString(s string) bool

	// TODO: Match(b []byte) bool
}

// CompileMatcher returns an optimized matcher for a given regular expression.
func CompileMatcher(expr string) (Matcher, error) {
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil, err
	}
	if m := optimizedMatcher(expr, re); m != nil {
		return m, nil
	}
	return regexp.Compile(expr)
}
