package regex

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

var matcherConstructors = []func(regexpData) Matcher{
	regexpData.suffixLitMatcher,
}

func optimizedMatcher(expr string, re *syntax.Regexp) Matcher {
	d := regexpData{expr: expr, re: re}
	for _, ctor := range matcherConstructors {
		if m := ctor(d); m != nil {
			return m
		}
	}
	return nil
}

type regexpData struct {
	expr string
	re   *syntax.Regexp
}

func (d regexpData) suffixLitMatcher() Matcher {
	if d.re.Flags != 0 {
		return nil
	}
	if d.re.Op != syntax.OpConcat {
		return nil
	}
	last := d.re.Sub[len(d.re.Sub)-1]
	if last.Op != syntax.OpLiteral {
		return nil
	}

	toReverse := *d.re
	toReverse.Sub = toReverse.Sub[:len(toReverse.Sub)-1]
	reversed := reversedPattern(&toReverse)
	re, err := regexp.Compile("^" + reversed)
	if err != nil {
		return nil
	}

	return &suffixLitMatcher{re: re, suffix: string(last.Rune)}
}

type suffixLitMatcher struct {
	suffix string
	re     *regexp.Regexp
}

func (m *suffixLitMatcher) MatchString(s string) bool {
	for {
		i := strings.Index(s, m.suffix)
		if i == -1 {
			return false
		}
		if m.re.MatchReader(newReverseReader(s[:i])) {
			return true
		}
		s = s[i+len(m.suffix):]
	}
	return false
}
