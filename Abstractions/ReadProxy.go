package Abstractions

import (
	"io"
)

type ReadProxy struct {
	io.ReadCloser
	Total uint64
}

func (this *ReadProxy) Read(p []byte) (int, error) {
	n, err := this.ReadCloser.Read(p)
	this.Total += uint64(n)

	return n, err
}
func (this *ReadProxy) Close() error {
	return this.ReadCloser.Close()
}
