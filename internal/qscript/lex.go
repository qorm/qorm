package qscript

import (
	"fmt"
	"strings"
	"unicode"
)

// ---- lexer ----
//
// Mirrors the expression language's lexer (internal/expr) so the two never
// disagree on lexemes: identifiers absorb dots (state.piece.x is one token),
// strings use single or double quotes with backslash escapes, numbers are
// digit/dot runs validated at parse time. Adds '#'- and '//'-to-end-of-line
// comments and a per-token line number for diagnostics.

type tkind int

const (
	tEOF tkind = iota
	tNumber
	tString
	tIdent
	tOp
)

type token struct {
	kind tkind
	text string
	line int
}

func lex(src string) ([]token, error) {
	var toks []token
	r := []rune(src)
	i, line := 0, 1
	for i < len(r) {
		c := r[i]
		switch {
		case c == '\n':
			line++
			i++
		case unicode.IsSpace(c):
			i++
		case c == '#': // comment to end of line
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(r) && r[i+1] == '/': // //-style comment, same as '#'
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(r) && unicode.IsDigit(r[i+1])):
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			toks = append(toks, token{tNumber, string(r[i:j]), line})
			i = j
		case c == '\'' || c == '"':
			quote := c
			j := i + 1
			var sb strings.Builder
			for j < len(r) && r[j] != quote {
				if r[j] == '\n' {
					line++
				}
				if r[j] == '\\' && j+1 < len(r) {
					j++
				}
				sb.WriteRune(r[j])
				j++
			}
			if j >= len(r) {
				return nil, &Error{line, fmt.Sprintf("unterminated string literal (missing closing %q)", string(quote))}
			}
			toks = append(toks, token{tString, sb.String(), line})
			i = j + 1
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_' || r[j] == '$' || r[j] == '.') {
				j++
			}
			toks = append(toks, token{tIdent, string(r[i:j]), line})
			i = j
		default:
			two := ""
			if i+1 < len(r) {
				two = string(r[i : i+2])
			}
			switch two {
			case "&&", "||", "==", "!=", "<=", ">=":
				toks = append(toks, token{tOp, two, line})
				i += 2
			default:
				toks = append(toks, token{tOp, string(c), line})
				i++
			}
		}
	}
	toks = append(toks, token{tEOF, "", line})
	return toks, nil
}
