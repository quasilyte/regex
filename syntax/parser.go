package syntax

import (
	"fmt"
	"strings"
)

func NewParser() *Parser {
	var p Parser

	p.exprPool = make([]Expr, 256)

	for tok, op := range tok2op {
		if op != 0 {
			p.prefixParselets[tokenKind(tok)] = p.parsePrefixElementary
		}
	}

	// TODO: can we handle group parsing in a more elegant way?

	p.prefixParselets[tokLparen] = p.parseCapture
	p.prefixParselets[tokLparenName] = p.parseNamedCapture
	p.prefixParselets[tokLparenFlags] = p.parseGroupWithFlags

	p.prefixParselets[tokLbracket] = func(tok token) *Expr {
		return p.parseCharClass(OpCharClass, tok)
	}
	p.prefixParselets[tokLbracketCaret] = func(tok token) *Expr {
		return p.parseCharClass(OpNegCharClass, tok)
	}

	p.infixParselets[tokRepeat] = func(left *Expr, tok token) *Expr {
		repeatLit := p.newExpr(OpLiteral, tok.pos)
		return p.newExpr(OpRepeat, combinePos(left.Pos, tok.pos), left, repeatLit)
	}
	p.infixParselets[tokPlus] = func(left *Expr, tok token) *Expr {
		return p.newExpr(OpPlus, tok.pos, left)
	}
	p.infixParselets[tokStar] = func(left *Expr, tok token) *Expr {
		return p.newExpr(OpStar, tok.pos, left)
	}
	p.infixParselets[tokPipe] = func(left *Expr, tok token) *Expr {
		right := p.parseExpr(1)
		return p.newExpr(OpAlt, tok.pos, left, right)
	}
	p.infixParselets[tokConcat] = func(left *Expr, tok token) *Expr {
		if left.Op == OpConcat {
			right := p.parseExpr(2)
			left.Args = append(left.Args, *right)
			return left
		}
		right := p.parseExpr(2)
		return p.newExpr(OpConcat, tok.pos, left, right)
	}
	p.infixParselets[tokMinus] = p.parseMinus
	p.infixParselets[tokQuestion] = p.parseQuestion

	return &p
}

type Parser struct {
	out      Regexp
	lexer    lexer
	exprPool []Expr

	prefixParselets [256]prefixParselet
	infixParselets  [256]infixParselet

	charClass []Expr
	allocated uint
}

type prefixParselet func(token) *Expr

type infixParselet func(*Expr, token) *Expr

func (p *Parser) Parse(pattern string) (result *Regexp, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if err2, ok := r.(ParseError); ok {
			err = err2
			return
		}
		panic(r)
	}()

	p.lexer.Init(pattern)
	p.allocated = 0
	p.out.Source = pattern
	if pattern == "" {
		p.out.Expr = *p.newExpr(OpConcat, Position{})
	} else {
		p.out.Expr = *p.parseExpr(0)
	}

	return &p.out, nil
}

func (p *Parser) newExpr(op Operation, pos Position, args ...*Expr) *Expr {
	e := p.allocExpr()
	*e = Expr{
		Op:   op,
		Pos:  pos,
		Args: e.Args[:0],
	}
	for _, arg := range args {
		e.Args = append(e.Args, *arg)
	}
	return e
}

func (p *Parser) allocExpr() *Expr {
	if p.allocated < uint(len(p.exprPool)) {
		p.allocated++
		return &p.exprPool[p.allocated]
	}
	return &Expr{}
}

func (p *Parser) expect(kind tokenKind) {
	tok := p.lexer.NextToken()
	if tok.kind != kind {
		throwErrorf(int(tok.pos.Begin), int(tok.pos.End), "expected '%s', found '%s'", kind, tok.kind)
	}
}

func (p *Parser) parseExpr(precedence int) *Expr {
	tok := p.lexer.NextToken()
	prefix := p.prefixParselets[tok.kind]
	if prefix == nil {
		panic(fmt.Errorf("unexpected token: %v", tok))
	}
	left := prefix(tok)

	for precedence < p.precedenceOf(p.lexer.Peek()) {
		tok := p.lexer.NextToken()
		infix := p.infixParselets[tok.kind]
		left = infix(left, tok)
	}

	return left
}

func (p *Parser) parsePrefixElementary(tok token) *Expr {
	return p.newExpr(tok2op[tok.kind], tok.pos)
}

func (p *Parser) parseCharClass(op Operation, tok token) *Expr {
	if p.lexer.Peek().kind == tokRbracket {
		// An empty char class `[]` or `[^]`. Not valid, but we take it.
		tok2 := p.lexer.NextToken()
		return p.newExpr(op, combinePos(tok.pos, tok2.pos))
	}

	var endPos Position
	p.charClass = p.charClass[:0]
	for {
		p.charClass = append(p.charClass, *p.parseExpr(0))
		next := p.lexer.Peek()
		if next.kind == tokRbracket {
			endPos = next.pos
			p.lexer.NextToken()
			break
		}
		if next.kind == tokNone {
			throwfPos(tok.pos, "unterminated '['")
		}
	}

	result := p.newExpr(op, combinePos(tok.pos, endPos))
	result.Args = append(result.Args, p.charClass...)
	return result
}

func (p *Parser) parseMinus(left *Expr, tok token) *Expr {
	switch left.Op {
	case OpEscapeMeta, OpLiteral:
		if next := p.lexer.Peek().kind; next == tokChar || next == tokEscapeMeta || next == tokMinus {
			right := p.parseExpr(2)
			return p.newExpr(OpCharRange, tok.pos, left, right)
		}
	}
	p.charClass = append(p.charClass, *left)
	return p.newExpr(OpLiteral, tok.pos)
}

func (p *Parser) parseQuestion(left *Expr, tok token) *Expr {
	op := OpQuestion
	switch left.Op {
	case OpPlus, OpStar, OpQuestion, OpRepeat:
		op = OpNonGreedy
	}
	return p.newExpr(op, tok.pos, left)
}

func (p *Parser) parseGroupItem(tok token) *Expr {
	if p.lexer.Peek().kind == tokRparen {
		return p.newExpr(OpConcat, Position{})
	}
	return p.parseExpr(0)
}

func (p *Parser) parseCapture(tok token) *Expr {
	x := p.parseGroupItem(tok)
	result := p.newExpr(OpCapture, tok.pos, x)
	p.expect(tokRparen)
	return result
}

func (p *Parser) parseNamedCapture(tok token) *Expr {
	name := p.newExpr(OpLiteral, Position{
		Begin: tok.pos.Begin + uint16(len("(?P<")),
		End:   tok.pos.End - uint16(len(">")),
	})
	x := p.parseGroupItem(tok)
	result := p.newExpr(OpNamedCapture, tok.pos, x, name)
	p.expect(tokRparen)
	return result
}

func (p *Parser) parseGroupWithFlags(tok token) *Expr {
	var result *Expr
	val := p.out.Source[tok.pos.Begin+1 : tok.pos.End]
	switch {
	case !strings.HasSuffix(val, ":"):
		flags := p.newExpr(OpLiteral, Position{
			Begin: tok.pos.Begin + uint16(len("(")),
			End:   tok.pos.End,
		})
		pos2 := p.lexer.Peek().pos
		result = p.newExpr(OpFlagOnlyGroup, combinePos(tok.pos, pos2), flags)
	case val == "?:":
		x := p.parseGroupItem(tok)
		result = p.newExpr(OpGroup, tok.pos, x)
	default:
		flags := p.newExpr(OpLiteral, Position{
			Begin: tok.pos.Begin + uint16(len("(")),
			End:   tok.pos.End - uint16(len(":")),
		})
		x := p.parseGroupItem(tok)
		result = p.newExpr(OpGroupWithFlags, tok.pos, x, flags)
	}
	p.expect(tokRparen)
	return result
}

func (p *Parser) precedenceOf(tok token) int {
	switch tok.kind {
	case tokPipe:
		return 1
	case tokConcat:
		return 2
	case tokPlus, tokStar, tokQuestion, tokMinus, tokRepeat:
		return 3
	default:
		return 0
	}
}

var tok2op = [256]Operation{
	tokDollar:        OpDollar,
	tokCaret:         OpCaret,
	tokDot:           OpDot,
	tokChar:          OpLiteral,
	tokMinus:         OpLiteral,
	tokEscape:        OpEscape,
	tokEscapeMeta:    OpEscapeMeta,
	tokEscapeHex:     OpEscapeHex,
	tokEscapeHexFull: OpEscapeHexFull,
	tokEscapeOctal:   OpEscapeOctal,
	tokEscapeUni:     OpEscapeUni,
	tokEscapeUniFull: OpEscapeUniFull,
	tokPosixClass:    OpPosixClass,
	tokQ:             OpQuote,
}
