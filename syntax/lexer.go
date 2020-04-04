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
	tokEscapeMeta
	tokEscapeOctal
	tokEscapeUni
	tokEscapeUniFull
	tokEscapeHex
	tokEscapeHexFull
	tokComment

	tokQ                        // \Q
	tokMinus                    // -
	tokLbracket                 // [
	tokLbracketCaret            // [^
	tokRbracket                 // ]
	tokDollar                   // $
	tokCaret                    // ^
	tokQuestion                 // ?
	tokDot                      // .
	tokPlus                     // +
	tokStar                     // *
	tokPipe                     // |
	tokLparen                   // (
	tokLparenName               // (?P<name>
	tokLparenFlags              // (?flags
	tokLparenAtomic             // (?>
	tokLparenPositiveLookahead  // (?=
	tokLparenPositiveLookbehind // (?<=
	tokLparenNegativeLookahead  // (?!
	tokLparenNegativeLookbehind // (?<!
	tokRparen                   // )
)

// reMetachar is a table of meta chars outside of a char class.
var reMetachar = [256]bool{
	'\\': true,
	'|':  true,
	'*':  true,
	'+':  true,
	'?':  true,
	'.':  true,
	'[':  true,
	']':  true,
	'^':  true,
	'$':  true,
	'(':  true,
	')':  true,
}

// charClassMetachar is a table of meta chars inside char class.
var charClassMetachar = [256]bool{
	'-': true,
	']': true,
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

func (l *lexer) scan() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch >= 128 {
			_, size := utf8.DecodeRuneInString(l.input[l.pos:])
			l.pushTok(tokChar, size)
			l.maybeInsertConcat()
			continue
		}
		switch ch {
		case '\\':
			l.scanEscape(false)
		case '.':
			l.pushTok(tokDot, 1)
		case '+':
			l.pushTok(tokPlus, 1)
		case '*':
			l.pushTok(tokStar, 1)
		case '^':
			l.pushTok(tokCaret, 1)
		case '$':
			l.pushTok(tokDollar, 1)
		case '?':
			l.pushTok(tokQuestion, 1)
		case ')':
			l.pushTok(tokRparen, 1)
		case '|':
			l.pushTok(tokPipe, 1)
		case '[':
			if l.byteAt(l.pos+1) == '^' {
				l.pushTok(tokLbracketCaret, 2)
			} else {
				l.pushTok(tokLbracket, 1)
			}
			l.scanCharClass()
		case '(':
			if l.byteAt(l.pos+1) == '?' {
				switch {
				case l.byteAt(l.pos+2) == '>':
					l.pushTok(tokLparenAtomic, len("(?>"))
				case l.byteAt(l.pos+2) == '=':
					l.pushTok(tokLparenPositiveLookahead, len("(?="))
				case l.byteAt(l.pos+2) == '!':
					l.pushTok(tokLparenNegativeLookahead, len("(?!"))
				case l.byteAt(l.pos+2) == '<' && l.byteAt(l.pos+3) == '=':
					l.pushTok(tokLparenPositiveLookbehind, len("(?<="))
				case l.byteAt(l.pos+2) == '<' && l.byteAt(l.pos+3) == '!':
					l.pushTok(tokLparenNegativeLookbehind, len("(?<!"))
				default:
					if j := l.commentWidth(l.pos + 1); j >= 0 {
						l.pushTok(tokComment, len("(")+j)
					} else if j = l.captureNameWidth(l.pos + 1); j >= 0 {
						l.pushTok(tokLparenName, len("(")+j)
					} else if j = l.groupFlagsWidth(l.pos + 1); j >= 0 {
						l.pushTok(tokLparenFlags, len("(")+j)
					} else {
						throwErrorf(l.pos, l.pos+1, "group token is incomplete")
					}
				}
			} else {
				l.pushTok(tokLparen, 1)
			}
		case '{':
			if j := l.repeatWidth(l.pos + 1); j >= 0 {
				l.pushTok(tokRepeat, len("{")+j)
			} else {
				l.pushTok(tokChar, 1)
			}
		default:
			l.pushTok(tokChar, 1)
		}
		l.maybeInsertConcat()
	}
}

func (l *lexer) scanCharClass() {
	l.maybeInsertConcat()
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch >= 128 {
			_, size := utf8.DecodeRuneInString(l.input[l.pos:])
			l.pushTok(tokChar, size)
			continue
		}
		switch ch {
		case '\\':
			l.scanEscape(true)
		case '[':
			isPosixClass := false
			if l.byteAt(l.pos+1) == ':' {
				j := l.stringIndex(l.pos+2, ":]")
				if j >= 0 {
					isPosixClass = true
					l.pushTok(tokPosixClass, j+len("[::]"))
				}
			}
			if !isPosixClass {
				l.pushTok(tokChar, 1)
			}
		case '-':
			l.pushTok(tokMinus, 1)
		case ']':
			l.pushTok(tokRbracket, 1)
			return // Stop scanning in the char context
		default:
			l.pushTok(tokChar, 1)
		}
	}
}

func (l *lexer) scanEscape(insideCharClass bool) {
	s := l.input
	if l.pos+1 >= len(s) {
		throwErrorf(l.pos, l.pos+1, `unexpected end of pattern: trailing '\'`)
	}
	switch {
	case s[l.pos+1] == 'p' || s[l.pos+1] == 'P':
		if l.pos+2 >= len(s) {
			throwErrorf(l.pos, l.pos+2, "unexpected end of pattern: expected uni-class-short or '{'")
		}
		if s[l.pos+2] == '{' {
			j := strings.IndexByte(s[l.pos+2:], '}')
			if j < 0 {
				throwErrorf(l.pos, l.pos+2, "can't find closing '}'")
			}
			l.pushTok(tokEscapeUniFull, len(`\p{`)+j)
		} else {
			l.pushTok(tokEscapeUni, len(`\pL`))
		}
	case s[l.pos+1] == 'x':
		if l.pos+2 >= len(s) {
			throwErrorf(l.pos, l.pos+2, "unexpected end of pattern: expected hex-digit or '{'")
		}
		if s[l.pos+2] == '{' {
			j := strings.IndexByte(s[l.pos+2:], '}')
			if j < 0 {
				throwErrorf(l.pos, l.pos+2, "can't find closing '}'")
			}
			l.pushTok(tokEscapeHexFull, len(`\x{`)+j)
		} else {
			if isHexDigit(l.byteAt(l.pos + 3)) {
				l.pushTok(tokEscapeHex, len(`\xFF`))
			} else {
				l.pushTok(tokEscapeHex, len(`\xF`))
			}
		}
	case isOctalDigit(s[l.pos+1]):
		digits := 1
		if isOctalDigit(l.byteAt(l.pos + 2)) {
			if isOctalDigit(l.byteAt(l.pos + 3)) {
				digits = 3
			} else {
				digits = 2
			}
		}
		l.pushTok(tokEscapeOctal, len(`\`)+digits)
	case s[l.pos+1] == 'Q':
		size := len(s) - l.pos // Until the pattern ends
		j := l.stringIndex(l.pos+2, `\E`)
		if j >= 0 {
			size = j + len(`\Q\E`)
		}
		l.pushTok(tokQ, size)

	default:
		kind := tokEscape
		if insideCharClass {
			if charClassMetachar[l.byteAt(l.pos+1)] {
				kind = tokEscapeMeta
			}
		} else {
			if reMetachar[l.byteAt(l.pos+1)] {
				kind = tokEscapeMeta
			}
		}
		l.pushTok(kind, 2)
	}
}

func (l *lexer) maybeInsertConcat() {
	if l.isConcatPos() {
		last := len(l.tokens) - 1
		tok := l.tokens[last]
		l.tokens[last].kind = tokConcat
		l.tokens = append(l.tokens, tok)
	}
}

func (l *lexer) Init(s string) {
	l.pos = 0
	l.tokens = l.tokens[:0]
	l.input = s

	l.scan()

	l.pos = 0
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
	colonPos := strings.IndexByte(l.input[pos:], ':')
	parenPos := strings.IndexByte(l.input[pos:], ')')
	if parenPos < 0 {
		return -1
	}
	if colonPos >= 0 && colonPos < parenPos {
		return colonPos + len(":")
	}
	return parenPos
}

func (l *lexer) commentWidth(pos int) int {
	if l.byteAt(pos) != '?' || l.byteAt(pos+1) != '#' {
		return -1
	}
	parenPos := strings.IndexByte(l.input[pos:], ')')
	if parenPos < 0 {
		return -1
	}
	return parenPos + len(`)`)
}

func (l *lexer) repeatWidth(pos int) int {
	j := pos
	for isDigit(l.byteAt(j)) {
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
	for isDigit(l.byteAt(j)) {
		j++
	}
	if l.byteAt(j) == '}' {
		return (j + len("}")) - pos // {min,} or {min,max}
	}
	return -1
}

func (l *lexer) stringIndex(offset int, s string) int {
	if offset < len(l.input) {
		return strings.Index(l.input[offset:], s)
	}
	return -1
}

func (l *lexer) byteAt(pos int) byte {
	if pos >= 0 && pos < len(l.input) {
		return l.input[pos]
	}
	return 0
}

func (l *lexer) pushTok(kind tokenKind, size int) {
	l.tokens = append(l.tokens, token{
		kind: kind,
		pos:  Position{Begin: uint16(l.pos), End: uint16(l.pos + size)},
	})
	l.pos += size
}

func (l *lexer) isConcatPos() bool {
	if len(l.tokens) < 2 {
		return false
	}
	x := l.tokens[len(l.tokens)-2].kind
	if concatTable[x]&concatX != 0 {
		return false
	}
	y := l.tokens[len(l.tokens)-1].kind
	return concatTable[y]&concatY == 0
}

const (
	concatX byte = 1 << iota
	concatY
)

var concatTable = [256]byte{
	tokPipe: concatX | concatY,

	tokLparen:                   concatX,
	tokLparenFlags:              concatX,
	tokLparenName:               concatX,
	tokLparenAtomic:             concatX,
	tokLbracket:                 concatX,
	tokLparenPositiveLookahead:  concatX,
	tokLparenPositiveLookbehind: concatX,
	tokLparenNegativeLookahead:  concatX,
	tokLparenNegativeLookbehind: concatX,

	tokRparen:   concatY,
	tokRbracket: concatY,
	tokPlus:     concatY,
	tokStar:     concatY,
	tokQuestion: concatY,
	tokRepeat:   concatY,
}
