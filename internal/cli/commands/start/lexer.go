package start

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Tokenize parses a command line string into arguments, handling quotes and escapes.
// It does NOT support shell features like globbing or env expansion.
func Tokenize(input string) ([]string, error) {
	l := &lexer{input: []rune(input)}
	return l.tokenize()
}

type lexer struct {
	input []rune
	pos   int
	args  []string
	cur   strings.Builder
	state int
}

const (
	stateNormal = iota
	stateSingle
	stateDouble
)

func (l *lexer) tokenize() ([]string, error) {
	for l.pos = 0; l.pos < len(l.input); l.pos++ {
		r := l.input[l.pos]
		var err error
		switch l.state {
		case stateNormal:
			l.handleNormal(r)
		case stateSingle:
			l.handleSingle(r)
		case stateDouble:
			err = l.handleDouble(r)
		}
		if err != nil {
			return nil, err
		}
	}
	if l.state != stateNormal {
		return nil, errors.New("unclosed quote")
	}
	if l.cur.Len() > 0 {
		l.args = append(l.args, l.cur.String())
	}
	return l.args, nil
}

func (l *lexer) handleNormal(r rune) {
	switch {
	case unicode.IsSpace(r):
		if l.cur.Len() > 0 {
			l.args = append(l.args, l.cur.String())
			l.cur.Reset()
		}
	case r == '\'':
		l.state = stateSingle
	case r == '"':
		l.state = stateDouble
	default:
		l.cur.WriteRune(r)
	}
}

func (l *lexer) handleSingle(r rune) {
	if r == '\'' {
		l.state = stateNormal
	} else {
		l.cur.WriteRune(r)
	}
}

func (l *lexer) handleDouble(r rune) error {
	switch r {
	case '"':
		l.state = stateNormal
	case '\\':
		return l.handleEscape()
	default:
		l.cur.WriteRune(r)
	}
	return nil
}

func (l *lexer) handleEscape() error {
	if l.pos+1 >= len(l.input) {
		return errors.New("invalid escape sequence: trailing backslash")
	}
	next := l.input[l.pos+1]
	switch next {
	case '"', '\\':
		l.cur.WriteRune(next)
		l.pos++ // skip next
		return nil
	default:
		return fmt.Errorf("invalid escape sequence: \\%c", next)
	}
}
