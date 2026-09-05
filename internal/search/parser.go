// Package search parses user search queries into FTS5 MATCH expressions.
// It supports field-qualified terms (name:, tag:, desc:), quoted phrases,
// and boolean operators (AND, OR, NOT). All values are quoted to prevent
// FTS5 syntax injection.
package search

import (
	"errors"
	"strings"
	"unicode"
)

// ErrEmptyQuery is returned when the input contains no searchable terms.
var ErrEmptyQuery = errors.New("search: empty query")

// fieldAliases maps user-facing field prefixes to FTS5 column names.
var fieldAliases = map[string]string{
	"name":        "name",
	"tag":         "tags",
	"tags":        "tags",
	"desc":        "description",
	"description": "description",
}

// token represents a parsed element of the search input.
type token struct {
	field string // FTS5 column name, empty for unqualified terms
	value string // the search term or phrase
	op    string // boolean operator: AND, OR, NOT (mutually exclusive with value)
}

// Parse converts a user query string into an FTS5 MATCH expression.
// Field qualifiers (name:, tag:, desc:) are mapped to FTS5 columns.
// Unqualified terms match all columns. AND is implicit between adjacent terms.
func Parse(input string) (string, error) {
	tokens := tokenize(input)
	if len(tokens) == 0 {
		return "", ErrEmptyQuery
	}
	return build(tokens)
}

// tokenize splits input into structured tokens.
func tokenize(input string) []token {
	r := strings.NewReader(input)
	var tokens []token

	for {
		skipSpaces(r)
		if r.Len() == 0 {
			break
		}
		word := readWord(r)
		if word == "" {
			// Possibly a quote-only segment.
			if peekByte(r) == '"' {
				_, _ = r.ReadByte()
				phrase := readQuoted(r)
				if phrase != "" {
					tokens = append(tokens, token{value: phrase})
				}
				continue
			}
			// Skip unknown single character.
			_, _ = r.ReadByte()
			continue
		}

		// Check for boolean operator (uppercase only, not followed by colon).
		if isOperator(word) && peekByte(r) != ':' {
			tokens = append(tokens, token{op: word})
			continue
		}

		// Check for field qualifier (word:value).
		if col, ok := fieldAliases[strings.ToLower(word)]; ok && peekByte(r) == ':' {
			_, _ = r.ReadByte() // consume ':'
			skipSpaces(r)
			val := readValue(r)
			if val != "" {
				tokens = append(tokens, token{field: col, value: val})
			}
			continue
		}

		tokens = append(tokens, token{value: word})
	}
	return tokens
}

// build assembles tokens into an FTS5 MATCH string.
func build(tokens []token) (string, error) {
	var parts []string
	pendingOp := ""
	haveLeft := false

	for _, t := range tokens {
		if t.op != "" {
			if haveLeft {
				pendingOp = t.op
			}
			continue
		}
		rendered := renderTerm(t)
		if haveLeft {
			if pendingOp == "" {
				pendingOp = "AND"
			}
			parts = append(parts, pendingOp)
		}
		parts = append(parts, rendered)
		pendingOp = ""
		haveLeft = true
	}

	// Drop trailing operator (no right operand).
	result := strings.Join(parts, " ")
	if result == "" {
		return "", ErrEmptyQuery
	}
	return result, nil
}

// renderTerm formats a token as an FTS5 expression fragment.
func renderTerm(t token) string {
	quoted := ftsQuote(t.value)
	if t.field != "" {
		return t.field + " : " + quoted
	}
	return quoted
}

// ftsQuote wraps v in double quotes, escaping embedded quotes.
func ftsQuote(v string) string {
	escaped := strings.ReplaceAll(v, `"`, `""`)
	return `"` + escaped + `"`
}

// --- reader helpers ---

func skipSpaces(r *strings.Reader) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		if !unicode.IsSpace(rune(b)) {
			_ = r.UnreadByte()
			return
		}
	}
}

func peekByte(r *strings.Reader) byte {
	b, err := r.ReadByte()
	if err != nil {
		return 0
	}
	_ = r.UnreadByte()
	return b
}

// readWord reads a contiguous run of non-space, non-quote, non-colon bytes.
func readWord(r *strings.Reader) string {
	var sb strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if unicode.IsSpace(rune(b)) || b == '"' || b == ':' {
			_ = r.UnreadByte()
			break
		}
		sb.WriteByte(b)
	}
	return sb.String()
}

// readQuoted reads until the closing double-quote (or EOF).
func readQuoted(r *strings.Reader) string {
	var sb strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if b == '"' {
			break
		}
		sb.WriteByte(b)
	}
	return sb.String()
}

// readValue reads a quoted phrase or bareword after a field qualifier.
func readValue(r *strings.Reader) string {
	if peekByte(r) == '"' {
		_, _ = r.ReadByte()
		return readQuoted(r)
	}
	return readWord(r)
}

// isOperator returns true for uppercase boolean keywords.
func isOperator(word string) bool {
	return word == "AND" || word == "OR" || word == "NOT"
}
