package Discard

type DiscardCloser struct{}

func (DiscardCloser) Write(p []byte) (n int, err error) {
	return len(p), nil
}
func (DiscardCloser) Close() error { return nil }
