package Abstractions

import (
	"io"
)

type WriteProxy struct {
	io.WriteCloser
	Proxified io.WriteCloser
}

func (this *WriteProxy) Write(p []byte) (int, error) {
	n, err := this.Proxified.Write(p)

	return n, err
}
func (this *WriteProxy) Close() error {
	return this.Proxified.Close()
}
