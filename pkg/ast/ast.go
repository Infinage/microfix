package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
)

// Core AST node interface
type Matcher interface {
	Match(msg *message.Message) bool
}

// --- Leaf Node ---

type tagMatcher struct {
	Tag   uint16
	Value string
}

func (m *tagMatcher) Match(msg *message.Message) bool {
	val, ok := msg.Get(m.Tag)
	return ok && val == m.Value
}

// --- Logical Nodes ---

type andMatcher struct {
	Left  Matcher
	Right Matcher
}

func (m *andMatcher) Match(msg *message.Message) bool {
	return m.Left.Match(msg) && m.Right.Match(msg)
}

type orMatcher struct {
	Left  Matcher
	Right Matcher
}

func (m *orMatcher) Match(msg *message.Message) bool {
	return m.Left.Match(msg) || m.Right.Match(msg)
}

type notMatcher struct {
	Operand Matcher
}

func (m *notMatcher) Match(msg *message.Message) bool {
	return !m.Operand.Match(msg)
}

// --- Lexer (Tokenizer) ---

// tokenize breaks a raw string into proper logical tokens, respecting operators as boundaries.
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range input {
		switch r {
		case '&', '|', '!', '(', ')':
			// If we were building an operand (like "35=D"), push it first
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Push the operator
			tokens = append(tokens, string(r))
		case ' ', '\t':
			// Spaces just separate operands
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// --- Parser State ---

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) next() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

// --- Recursive Descent Parsing ---

// parseExpression handles OR (|) - Lowest Precedence
func (p *parser) parseExpression() (Matcher, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}

	for p.peek() == "|" {
		p.next() // consume '|'
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &orMatcher{Left: left, Right: right}
	}
	return left, nil
}

// parseTerm handles AND (&) - Medium Precedence
func (p *parser) parseTerm() (Matcher, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}

	for p.peek() == "&" {
		p.next() // consume '&'
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = &andMatcher{Left: left, Right: right}
	}
	return left, nil
}

// parseFactor handles NOT (!), Grouping (), and Tag=Value - Highest Precedence
func (p *parser) parseFactor() (Matcher, error) {
	t := p.next()
	if t == "" {
		return nil, fmt.Errorf("unexpected end of expression")
	}

	// Handle NOT
	if t == "!" {
		operand, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return &notMatcher{Operand: operand}, nil
	}

	// Handle Parentheses Grouping
	if t == "(" {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, fmt.Errorf("missing closing parenthesis ')'")
		}
		return expr, nil
	}

	// It must be an Operand (Tag=Value)
	parts := strings.SplitN(t, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid operand format, expected Tag=Value, got: '%s'", t)
	}

	tag, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid FIX tag '%s': %w", parts[0], err)
	}

	return &tagMatcher{Tag: uint16(tag), Value: parts[1]}, nil
}

// --- Public Entry Point ---

// NewMatcher takes raw arguments, tokenizes them correctly, and builds an AST.
// 35=D & (11=ORD1 | 11=ORD2) & !39=4
func NewMatcher(args []string) (Matcher, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no arguments provided")
	}

	// Rejoin the raw args (because strings.Fields mashed them up)
	rawExpression := strings.Join(args, " ")

	// Lex properly
	tokens := tokenize(rawExpression)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no valid tokens found")
	}

	// Parse starting at the lowest precedence (Expression -> OR)
	p := &parser{tokens: tokens}

	ast, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Ensure there are no leftover tokens
	if p.peek() != "" {
		return nil, fmt.Errorf("unexpected token at end of expression: '%s'", p.peek())
	}

	return ast, nil
}
