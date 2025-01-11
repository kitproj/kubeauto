package internal

import "io"

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

func funcWriter(f func([]byte) (int, error)) io.Writer {
	return writerFunc(f)
}
