package syntax

import (
	"fmt"
	"strings"
)

type ParserOptions struct {
	// NoLiterals disables OpChar merging into OpLiteral.
	NoLiterals bool
}

func NewParser(opts *ParserOptions) *Parser {
	var p Parser

	if opts != nil {
		p.opts = *opts
	}
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
		repeatLit := p.newExpr(OpString, tok.pos)
		return p.newExpr(OpRepeat, combinePos(left.Pos, tok.pos), left, repeatLit)
	}
	p.infixParselets[tokPlus] = func(left *Expr, tok token) *Expr {
		return p.newExpr(OpPlus, tok.pos, left)
	}
	p.infixParselets[tokStar] = func(left *Expr, tok token) *Expr {
		return p.newExpr(OpStar, tok.pos, left)
	}
	p.infixParselets[tokPipe] = func(left *Expr, tok token) *Expr {
		var right *Expr
		switch p.lexer.Peek().kind {
		case tokRparen, tokNone:
			right = p.newExpr(OpConcat, tok.pos)
		default:
			right = p.parseExpr(1)
		}
		if left.Op == OpAlt {
			left.Args = append(left.Args, *right)
			left.Pos.End = right.End()
			return left
		}
		return p.newExpr(OpAlt, combinePos(left.Pos, right.Pos), left, right)
	}
	p.infixParselets[tokConcat] = func(left *Expr, tok token) *Expr {
		right := p.parseExpr(2)
		if left.Op == OpConcat {
			left.Args = append(left.Args, *right)
			left.Pos.End = right.End()
			return left
		}
		return p.newExpr(OpConcat, combinePos(left.Pos, right.Pos), left, right)
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

	opts ParserOptions
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

	if !p.opts.NoLiterals {
		p.mergeChars(&p.out.Expr)
	}
	p.setValues(&p.out.Expr)

	return &p.out, nil
}

func (p *Parser) setValues(e *Expr) {
	for i := range e.Args {
		p.setValues(&e.Args[i])
	}
	e.Value = p.out.Source[e.Begin():e.End()]
}

func (p *Parser) mergeChars(e *Expr) {
	if e.Op != OpConcat || len(e.Args) < 2 {
		for i := range e.Args {
			p.mergeChars(&e.Args[i])
		}
		return
	}

	args := e.Args[:0]
	i := 0
	for i < len(e.Args) {
		first := i
		chars := 0
		for j := i; j < len(e.Args) && e.Args[j].Op == OpChar; j++ {
			chars++
		}
		if chars > 1 {
			c1 := e.Args[first]
			c2 := e.Args[first+chars-1]
			lit := p.newExpr(OpLiteral, combinePos(c1.Pos, c2.Pos))
			for j := 0; j < chars; j++ {
				lit.Args = append(lit.Args, e.Args[first+j])
			}
			args = append(args, *lit)
			i += chars
		} else {
			args = append(args, e.Args[i])
			i++
		}
	}
	if len(args) == 1 {
		*e = args[0] // Turn OpConcat into OpLiteral
	} else {
		e.Args = args
	}
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
	i := p.allocated
	if i < uint(len(p.exprPool)) {
		p.allocated++
		return &p.exprPool[i]
	}
	return &Expr{}
}

func (p *Parser) expect(kind tokenKind) Position {
	tok := p.lexer.NextToken()
	if tok.kind != kind {
		throwErrorf(int(tok.pos.Begin), int(tok.pos.End), "expected '%s', found '%s'", kind, tok.kind)
	}
	return tok.pos
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
	case OpEscapeMeta, OpChar:
		if next := p.lexer.Peek().kind; next == tokChar || next == tokEscapeMeta || next == tokMinus {
			right := p.parseExpr(2)
			return p.newExpr(OpCharRange, combinePos(left.Pos, right.Pos), left, right)
		}
	}
	p.charClass = append(p.charClass, *left)
	return p.newExpr(OpChar, tok.pos)
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
		return p.newExpr(OpConcat, tok.pos)
	}
	return p.parseExpr(0)
}

func (p *Parser) parseCapture(tok token) *Expr {
	x := p.parseGroupItem(tok)
	result := p.newExpr(OpCapture, tok.pos, x)
	result.Pos.End = p.expect(tokRparen).End
	return result
}

func (p *Parser) parseNamedCapture(tok token) *Expr {
	name := p.newExpr(OpString, Position{
		Begin: tok.pos.Begin + uint16(len("(?P<")),
		End:   tok.pos.End - uint16(len(">")),
	})
	x := p.parseGroupItem(tok)
	result := p.newExpr(OpNamedCapture, tok.pos, x, name)
	result.Pos.End = p.expect(tokRparen).End
	return result
}

func (p *Parser) parseGroupWithFlags(tok token) *Expr {
	var result *Expr
	val := p.out.Source[tok.pos.Begin+1 : tok.pos.End]
	switch {
	case !strings.HasSuffix(val, ":"):
		flags := p.newExpr(OpString, Position{
			Begin: tok.pos.Begin + uint16(len("(")),
			End:   tok.pos.End,
		})
		result = p.newExpr(OpFlagOnlyGroup, tok.pos, flags)
	case val == "?:":
		x := p.parseGroupItem(tok)
		result = p.newExpr(OpGroup, tok.pos, x)
	default:
		flags := p.newExpr(OpString, Position{
			Begin: tok.pos.Begin + uint16(len("(")),
			End:   tok.pos.End - uint16(len(":")),
		})
		x := p.parseGroupItem(tok)
		result = p.newExpr(OpGroupWithFlags, tok.pos, x, flags)
	}
	result.Pos.End = p.expect(tokRparen).End
	return result
}

func (p *Parser) precedenceOf(tok token) int {
	switch tok.kind {
	case tokPipe:
		return 1
	case tokConcat, tokMinus:
		return 2
	case tokPlus, tokStar, tokQuestion, tokRepeat:
		return 3
	default:
		return 0
	}
}

var tok2op = [256]Operation{
	tokDollar:        OpDollar,
	tokCaret:         OpCaret,
	tokDot:           OpDot,
	tokChar:          OpChar,
	tokMinus:         OpChar,
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
