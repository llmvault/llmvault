package databaseintegration

import (
	"fmt"
	"strings"
)

func normalizeSQL(provider, query string) (string, string, error) {
	src := []byte(query)
	n := len(src)
	exec := make([]byte, 0, n)
	mask := make([]byte, 0, n)
	backslash := provider == ProviderMySQL
	nested := provider == ProviderPostgres
	dollars := provider == ProviderPostgres
	i := 0
	for i < n {
		c := src[i]
		switch {
		case c == '\'':
			end, err := scanStringLiteral(src, i, backslash)
			if err != nil {
				return "", "", err
			}
			literal := src[i:end]
			exec = append(exec, literal...)
			mask = append(mask, maskLiteral(literal)...)
			i = end
		case c == '"':
			end, err := scanQuotedIdent(src, i)
			if err != nil {
				return "", "", err
			}
			exec = append(exec, src[i:end]...)
			mask = append(mask, src[i:end]...)
			i = end
		case c == '-' && i+1 < n && src[i+1] == '-':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			i = j
		case c == '/' && i+1 < n && src[i+1] == '*':
			end, err := scanBlockComment(src, i, nested)
			if err != nil {
				return "", "", err
			}
			i = end
		case dollars && c == '$':
			end, ok, err := scanDollarString(src, i)
			if err != nil {
				return "", "", err
			}
			if !ok {
				exec = append(exec, c)
				mask = append(mask, c)
				i++
				continue
			}
			exec = append(exec, src[i:end]...)
			mask = append(mask, spaces(end-i)...)
			i = end
		default:
			exec = append(exec, c)
			mask = append(mask, c)
			i++
		}
	}
	return string(exec), string(mask), nil
}

func scanStringLiteral(src []byte, start int, backslash bool) (int, error) {
	n := len(src)
	j := start + 1
	for j < n {
		ch := src[j]
		if backslash && ch == '\\' && j+1 < n {
			j += 2
			continue
		}
		if ch == '\'' {
			if j+1 < n && src[j+1] == '\'' {
				j += 2
				continue
			}
			return j + 1, nil
		}
		j++
	}
	return 0, fmt.Errorf("unterminated string literal")
}

func scanQuotedIdent(src []byte, start int) (int, error) {
	n := len(src)
	j := start + 1
	for j < n {
		if src[j] == '"' {
			if j+1 < n && src[j+1] == '"' {
				j += 2
				continue
			}
			return j + 1, nil
		}
		j++
	}
	return 0, fmt.Errorf("unterminated quoted identifier")
}

func scanBlockComment(src []byte, start int, nested bool) (int, error) {
	n := len(src)
	depth := 1
	j := start + 2
	for j < n && depth > 0 {
		if src[j] == '*' && j+1 < n && src[j+1] == '/' {
			depth--
			j += 2
			continue
		}
		if nested && src[j] == '/' && j+1 < n && src[j+1] == '*' {
			depth++
			j += 2
			continue
		}
		j++
	}
	if depth != 0 {
		return 0, fmt.Errorf("unterminated block comment")
	}
	return j, nil
}

func scanDollarString(src []byte, start int) (int, bool, error) {
	tag, ok := dollarTag(src, start)
	if !ok {
		return 0, false, nil
	}
	rest := string(src[start+len(tag):])
	idx := strings.Index(rest, tag)
	if idx < 0 {
		return 0, false, fmt.Errorf("unterminated dollar-quoted string")
	}
	return start + len(tag) + idx + len(tag), true, nil
}

func dollarTag(src []byte, start int) (string, bool) {
	n := len(src)
	j := start + 1
	for j < n {
		c := src[j]
		if c == '$' {
			return string(src[start : j+1]), true
		}
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			j++
			continue
		}
		return "", false
	}
	return "", false
}

func maskLiteral(literal []byte) []byte {
	out := make([]byte, len(literal))
	for i := range literal {
		if literal[i] == '\'' && (i == 0 || i == len(literal)-1) {
			out[i] = '\''
			continue
		}
		out[i] = ' '
	}
	return out
}

func spaces(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return out
}
