package regex

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
)

var inputChunk string

func init() {
	data, err := ioutil.ReadFile("./testdata/HACKING.md")
	if err != nil {
		panic(err)
	}
	inputChunk = strings.Repeat(string(data), 10)
}

type matcherTest struct {
	expr        string // Regexp being tested/benchmarked
	match       string // A string that matches tested regexp
	almostMatch string // Almost-matching string
}

var matcherTests = []*matcherTest{
	// Unbound head; Literal suffix.
	{expr: `[A-Z]+_SUSPEND`, match: "THREAD_SUSPEND", almostMatch: "123_SUSPEND"},
}

func BenchmarkMatcher(b *testing.B) {
	// To evaluate the results, run something like:
	//	go-benchrun /std /opt -count 10 .

	runSingleBench := func(name, input string, want bool, m Matcher) {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				have := m.MatchString(input)
				if have != want {
					b.Fatalf("unexpected result: have %v, want %v", have, want)
				}
			}
		})
	}

	for _, test := range matcherTests {
		// input contains a match inside middle part of the text.
		input := inputChunk + " " + test.match + " " + inputChunk
		// inputTiny is a minimal input that matches the pattern.
		inputTiny := test.match
		// inputNoMatch contains no matches at all.
		inputNoMatch := inputChunk
		// inputNoMatchTiny is a non-matching tiny input text.
		inputNoMatchTiny := "(@Qs_&^$^&*#^$(@*@#))"
		// inputHard contains a lot of almost matching substrings.
		inputHard := strings.Replace(inputChunk, "it", test.almostMatch, -1)

		re := regexp.MustCompile(test.expr)
		m, err := CompileMatcher(test.expr)
		if err != nil {
			b.Fatalf("compile(`%s`): %v", test.expr, err)
		}

		runBench := func(kind, input string, want bool) {
			nameTail := test.expr + "/" + kind + "/" + fmt.Sprint(len(input))
			runSingleBench("std/"+nameTail, input, want, re)
			runSingleBench("opt/"+nameTail, input, want, m)
		}

		runBench("match", input, true)
		runBench("match", inputTiny, true)
		runBench("nomatch", inputNoMatch, false)
		runBench("nomatch", inputNoMatchTiny, false)
		runBench("almost", inputHard, false)
	}
}

func TestSuffixLitMatcher(t *testing.T) {
	tests := []struct {
		expr    string
		match   []string
		nomatch []string
	}{
		{
			expr: `[A-Z]+_SUSPEND`,
			match: []string{
				`A_SUSPEND`,
				` FOO_SUSPEND`,
				`FOO_SUSPEND `,
				` A_SUSPENDED `,
			},
			nomatch: []string{
				`_SUSPEND`,
				` _SUSPEND`,
				`_SUSPEND `,
				` _SUSPEND`,
				`a_SUSPEND`,
				`linux_suspend`,
				`A _SUSPEND`,
			},
		},
	}

	for _, test := range tests {
		m, err := CompileMatcher(test.expr)
		if err != nil {
			t.Fatalf("compile(`%s`): %v", test.expr, err)
		}
		if _, ok := m.(*suffixLitMatcher); !ok {
			t.Errorf("compile(`%s`): expected *suffixLitMatcher, got %T", test.expr, m)
			continue
		}
		for _, s := range test.match {
			if !m.MatchString(s) {
				t.Errorf("match(`%s`): not matched", s)
			}
		}
		for _, s := range test.nomatch {
			if m.MatchString(s) {
				t.Errorf("match(`%s`): matched", s)
			}
		}
	}
}
