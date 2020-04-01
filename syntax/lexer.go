package syntax

import (
	"strings"
	"unicode/utf8"
)

type token struct {
	kind tokenKind
	pos  Position
}

func (tok token) String() string {
	return tok.kind.String()
}

type tokenKind byte

//go:generate stringer -type=tokenKind -trimprefix=tok -linecomment=true
const (
	tokNone tokenKind = iota

	tokChar
	tokGroupFlags
	tokPosixClass
	tokConcat
	tokRepeat
	tokEscape
	tokEscapeChar
	tokEscapeOctal
	tokEscapeUni
	tokEscapeUniFull
	tokEscapeHex
	tokEscapeHexFull

	tokQ             // \Q
	tokMinus         // -
	tokLbracket      // [
	tokLbracketCaret // [^
	tokRbracket      // ]
	tokDollar        // $
	tokCaret         // ^
	tokQuestion      // ?
	tokDot           // .
	tokPlus          // +
	tokStar          // *
	tokPipe          // |
	tokLparen        // (
	tokLparenName    // (?P<name>
	tokLparenFlags   // (?flags
	tokRparen        // )
)

// reMetachar is a table of meta chars outside of a char class.
var reMetachar = [256]bool{
	'\\': true,
	'|':  true,
	'*':  true,
	'+':  true,
	'.':  true,
	'[':  true,
	'$':  true,
	'(':  true,
	')':  true,
}

// charClassMetachar is a table of meta chars inside char class.
var charClassMetachar = [256]bool{
	']': true,
	'-': true,

	// ...plus all chars from the reMetachar.
	'\\': true,
	'|':  true,
	'*':  true,
	'+':  true,
	'.':  true,
	'[':  true,
	'$':  true,
	'(':  true,
	')':  true,
}

type lexer struct {
	tokens []token
	pos    int
	input  string
}

func (l *lexer) HasMoreTokens() bool {
	return l.pos < len(l.tokens)
}

func (l *lexer) NextToken() token {
	if l.pos < len(l.tokens) {
		tok := l.tokens[l.pos]
		l.pos++
		return tok
	}
	return token{}
}

func (l *lexer) Peek() token {
	if l.pos < len(l.tokens) {
		return l.tokens[l.pos]
	}
	return token{}
}

func (l *lexer) Init(s string) error {
	l.pos = 0
	l.tokens = l.tokens[:0]
	l.input = s

	i := 0
	size := 0
	insideCharClass := false
	pushTok := func(kind tokenKind) {
		l.tokens = append(l.tokens, token{
			kind: kind,
			pos:  Position{Begin: uint16(i), End: uint16(i + size)},
		})
	}
	pushMetaTok := func(kind tokenKind) {
		if insideCharClass {
			pushTok(tokChar)
		} else {
			pushTok(kind)
		}
	}

	for i < len(s) {
		var ch rune
		ch, size = utf8.DecodeRuneInString(s[i:])

		switch ch {
		case '.':
			pushMetaTok(tokDot)
		case '+':
			pushMetaTok(tokPlus)
		case '*':
			pushMetaTok(tokStar)
		case '^':
			pushMetaTok(tokCaret)
		case '$':
			pushMetaTok(tokDollar)
		case '?':
			pushMetaTok(tokQuestion)

		case '(':
			if insideCharClass {
				pushTok(tokChar)
			} else {
				if j := l.captureNameWidth(i + 1); j >= 0 {
					size += j
					pushTok(tokLparenName)
				} else if j = l.groupFlagsWidth(i + 1); j >= 0 {
					size += j
					pushTok(tokLparenFlags)
				} else {
					pushTok(tokLparen)
				}
			}
		case ')':
			pushMetaTok(tokRparen)
		case '|':
			pushMetaTok(tokPipe)

		case '{':
			j := l.repeatWidth(i + 1)
			if j >= 0 {
				size += j
				pushTok(tokRepeat)
			} else {
				pushTok(tokChar)
			}

		case '-':
			if insideCharClass {
				pushTok(tokMinus)
			} else {
				pushTok(tokChar)
			}
		case '[':
			if insideCharClass {
				isPosixClass := false
				if l.byteAt(i+1) == ':' && i+2 < len(s) {
					j := strings.Index(s[i+2:], ":]")
					if j >= 0 {
						isPosixClass = true
						size += j + len(":") + len(":]")
						pushTok(tokPosixClass)
					}
				}
				if !isPosixClass {
					pushTok(tokChar)
				}
			} else {
				if l.byteAt(i+1) == '^' {
					size++
					pushTok(tokLbracketCaret)
				} else {
					pushTok(tokLbracket)
				}
			}
		case ']':
			if insideCharClass {
				pushTok(tokRbracket)
			} else {
				pushTok(tokChar)
			}

		case '\\':
			if i+1 >= len(s) {
				throwErrorf(i, i+1, `unexpected end of pattern: trailing '\'`)
			}
			switch {
			case s[i+1] == 'p' || s[i+1] == 'P':
				if i+2 >= len(s) {
					throwErrorf(i, i+2, "unexpected end of pattern: expected uni-class-short or '{'")
				}
				if s[i+2] == '{' {
					j := strings.IndexByte(s[i+2:], '}')
					if j < 0 {
						throwErrorf(i, i+2, "can't find closing '}'")
					}
					size += j + len("p{")
					pushTok(tokEscapeUniFull)
				} else {
					size += 2
					pushTok(tokEscapeUni)
				}
			case s[i+1] == 'x':
				if i+2 >= len(s) {
					throwErrorf(i, i+2, "unexpected end of pattern: expected hex-digit or '{'")
				}
				if s[i+2] == '{' {
					j := strings.IndexByte(s[i+2:], '}')
					if j < 0 {
						throwErrorf(i, i+2, "can't find closing '}'")
					}
					size += j + len("x{")
					pushTok(tokEscapeHexFull)
				} else {
					size += 3
					pushTok(tokEscapeHex)
				}
			case l.isOctalDigit(s[i+1]):
				size++
				if l.isOctalDigit(l.byteAt(i + 2)) {
					size++
				}
				if l.isOctalDigit(l.byteAt(i + 3)) {
					size++
				}
				pushTok(tokEscapeOctal)
			case s[i+1] == 'Q':
				j := -1
				if i+2 < len(s) {
					j = strings.Index(s[i+2:], `\E`)
				}
				if j == -1 {
					size = len(s) - i
				} else {
					size = j + len(`\Q\E`)
				}
				pushTok(tokQ)

			default:
				size++
				kind := tokEscape
				if insideCharClass {
					if charClassMetachar[l.byteAt(i+1)] {
						kind = tokEscapeChar
					}
				} else {
					if reMetachar[l.byteAt(i+1)] {
						kind = tokEscapeChar
					}
				}
				pushTok(kind)
			}

		default:
			pushTok(tokChar)
		}

		if !insideCharClass && l.isConcatPos() {
			last := len(l.tokens) - 1
			tok := l.tokens[last]
			l.tokens[last].kind = tokConcat
			l.tokens = append(l.tokens, tok)
		}

		switch {
		case ch == '[':
			insideCharClass = true
		case ch == ']':
			insideCharClass = false
		}

		i += size
	}

	return nil
}

func (l *lexer) captureNameWidth(pos int) int {
	if !strings.HasPrefix(l.input[pos:], "?P<") {
		return -1
	}
	end := strings.IndexByte(l.input[pos:], '>')
	if end >= 0 {
		return end + len(">")
	}
	return -1
}

func (l *lexer) groupFlagsWidth(pos int) int {
	if l.byteAt(pos) != '?' {
		return -1
	}
	end := strings.IndexByte(l.input[pos:], ':')
	if end >= 0 {
		return end + len(":")
	}
	end = strings.IndexByte(l.input[pos:], ')')
	if end >= 0 {
		return end
	}
	return -1
}

func (l *lexer) repeatWidth(pos int) int {
	j := pos
	for l.isDigit(l.byteAt(j)) {
		j++
	}
	if j == pos {
		return -1
	}
	if l.byteAt(j) == '}' {
		return (j + len("}")) - pos // {min}
	}
	if l.byteAt(j) != ',' {
		return -1
	}
	j += len(",")
	for l.isDigit(l.byteAt(j)) {
		j++
	}
	if l.byteAt(j) == '}' {
		return (j + len("}")) - pos // {min,} or {min,max}
	}
	return -1
}

func (l *lexer) byteAt(pos int) byte {
	if pos >= 0 && pos < len(l.input) {
		return l.input[pos]
	}
	return 0
}

func (l *lexer) isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func (l *lexer) isOctalDigit(ch byte) bool {
	return ch >= '0' && ch <= '7'
}

func (l *lexer) isConcatPos() bool {
	if len(l.tokens) < 2 {
		return false
	}

	// TODO: find a better way to find a concat pos.

	x := l.tokens[len(l.tokens)-2].kind
	y := l.tokens[len(l.tokens)-1].kind
	switch {
	case x == tokLparen || x == tokLparenFlags || x == tokLparenName || x == tokLbracket:
		return false
	case x == tokPipe || y == tokPipe:
		return false
	case y == tokRparen || y == tokRbracket || y == tokPlus || y == tokStar || y == tokQuestion || y == tokRepeat:
		return false
	default:
		return true
	}
}
