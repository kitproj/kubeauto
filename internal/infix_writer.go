package internal

import (
	"bytes"
	"fmt"
)

type infixWriter struct {
	prefix string
	suffix string
	buffer bytes.Buffer
}

func (w *infixWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			fmt.Printf("%s%s%s\n", w.prefix, w.buffer.String(), w.suffix)
			w.buffer.Reset()
		} else {
			w.buffer.WriteByte(b)
		}
	}

	return len(p), nil
}
