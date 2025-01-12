package internal

import (
	"bytes"
	"fmt"
	"io"
)

// https://github.com/gawin/bash-colors-256
// not too dark or light
var colors = []int{
	34, 35, 36, 37, 38, 39,
	70, 71, 72, 73, 74, 75,
	106, 107, 108, 109, 110, 111,
	142, 143, 144, 145, 146, 147,
	178, 179, 180, 181, 182, 183,
	214, 215, 216, 217, 218, 219,
}

// colorize the text based on the name
func color(name, text string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code(name), text)
}

func code(name string) int {
	code := 0
	for _, r := range name {
		code += int(r)
	}
	return colors[code%len(colors)]
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
