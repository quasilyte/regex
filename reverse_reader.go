package regex

import (
	"io"
	"unicode/utf8"
)

type reverseReader struct {
	s string
	i int // current reading index
}

func (rr *reverseReader) ReadRune() (rune, int, error) {
	if rr.i < 0 {
		return 0, 0, io.EOF
	}
	if c := rr.s[rr.i]; c < utf8.RuneSelf {
		rr.i--
		return rune(c), 1, nil
	}
	ch, size := utf8.DecodeLastRuneInString(rr.s[:rr.i+1])
	rr.i -= int(size)
	return ch, size, nil
}

func newReverseReader(s string) *reverseReader {
	return &reverseReader{s, len(s) - 1}
}
