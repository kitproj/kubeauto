package internal

import (
	"bytes"
	"fmt"
	"io"
)

// colorize the text based on the name
func color(name, text string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code(name), text)
}

func code(name string) int {
	code := 0
	for _, r := range name {
		code += int(r)
	}
	// 16 to 231
	return 16 + code%216
}

func colorWriter(name, prefix string) io.Writer {
	return funcWriter(func(p []byte) (n int, err error) {
		// each line should with
		// - the color escape code
		// - the pod and container name
		// - the log line
		// each line should end  with a reset escape
		// p may contain newlines, so cater for that too
		lines := bytes.Split(p, []byte("\n"))
		for i, line := range lines {
			if len(line) == 0 {
				continue
			}
			if i == len(lines)-1 {
				fmt.Printf(color(name, prefix+string(line)))
			} else {
				fmt.Printf(color(name, prefix+string(line)+"\n"))
			}
		}

		return len(p), nil
	})
}
