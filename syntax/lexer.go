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
		case ')':
			pushMetaTok(tokRparen)
		case '|':
			pushMetaTok(tokPipe)

		case '(':
			if insideCharClass {
				pushTok(tokChar)
				break
			}
			if l.byteAt(i+1) == '?' {
				switch {
				case l.byteAt(i+2) == '>':
					size += len("?>")
					pushTok(tokLparenAtomic)
				case l.byteAt(i+2) == '=':
					size += len("?=")
					pushTok(tokLparenPositiveLookahead)
				case l.byteAt(i+2) == '!':
					size += len("?!")
					pushTok(tokLparenNegativeLookahead)
				case l.byteAt(i+2) == '<' && l.byteAt(i+3) == '=':
					size += len("?<=")
					pushTok(tokLparenPositiveLookbehind)
				case l.byteAt(i+2) == '<' && l.byteAt(i+3) == '!':
					size += len("?<!")
					pushTok(tokLparenNegativeLookbehind)
				default:
					if j := l.commentWidth(i + 1); j >= 0 {
						size += j
						pushTok(tokComment)
					} else if j = l.captureNameWidth(i + 1); j >= 0 {
						size += j
						pushTok(tokLparenName)
					} else if j = l.groupFlagsWidth(i + 1); j >= 0 {
						size += j
						pushTok(tokLparenFlags)
					} else {
						throwErrorf(i, i+1, "group token is incomplete")
					}
				}
			} else {
				pushTok(tokLparen)
			}

		case '{':
			if !insideCharClass {
				j := l.repeatWidth(i + 1)
				if j >= 0 {
					size += j
					pushTok(tokRepeat)
					break
				}
			}
			pushTok(tokChar)

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
					if isHexDigit(l.byteAt(i + 3)) {
						size += 3
					} else {
						size += 2
					}
					pushTok(tokEscapeHex)
				}
			case isOctalDigit(s[i+1]):
				size++
				if isOctalDigit(l.byteAt(i + 2)) {
					size++
				}
				if isOctalDigit(l.byteAt(i + 3)) {
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
						kind = tokEscapeMeta
					}
				} else {
					if reMetachar[l.byteAt(i+1)] {
						kind = tokEscapeMeta
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

func (l *lexer) byteAt(pos int) byte {
	if pos >= 0 && pos < len(l.input) {
		return l.input[pos]
	}
	return 0
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
