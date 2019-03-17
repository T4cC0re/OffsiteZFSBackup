package Abstractions

import (
	"io"
)

type WriteProxy struct {
	io.Writer
	Proxified io.Writer
}

func (this *WriteProxy) Write(p []byte) (int, error) {
	n, err := this.Proxified.Write(p)

	return n, err
}
