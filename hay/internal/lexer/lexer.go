package lexer

import (
	"github.com/metsitaba/voxgig-exp/hay/internal/token"
)

// Lexer performs lexical analysis on input source code.
type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position (after current char)
	ch           byte // current char under examination
	line         int
	column       int
}

// New creates a new Lexer for the given input string.
func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0}
	l.readChar()
	return l
}

// NextToken reads the next token from the input.
// Stub: returns EOF for now.
func (l *Lexer) NextToken() token.Token {
	return token.Token{Type: token.EOF, Literal: "", Line: l.line, Column: l.column}
}

// Tokenize returns all tokens from the input as a slice.
func (l *Lexer) Tokenize() []token.Token {
	var tokens []token.Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == token.EOF {
			break
		}
	}
	return tokens
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
}
