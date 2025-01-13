package internal

import "strings"

func delim(text, prefix, suffix string) string {
	if text == "" {
		return ""
	}
	return prefix + text + suffix
}

func join(texts ...string) string {
	builder := strings.Builder{}
	for i, text := range texts {
		builder.WriteString(text)
		if text != "" && i < len(texts)-1 {
			builder.WriteString(" ")
		}
	}
	return builder.String()
}
