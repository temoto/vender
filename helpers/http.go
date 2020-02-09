package helpers

import (
	"bufio"
	"bytes"
	"net/http"
)

type MockHTTP struct {
	Fun    func(*http.Request) (*http.Response, error)
	Header []byte
	Body   []byte
	Err    error
}

func (m *MockHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.Fun != nil {
		return m.Fun(req)
	}
	if m.Err != nil {
		return nil, m.Err
	}
	header := m.Header
	if header == nil {
		header = []byte("HTTP/1.0 200 OK\r\n\r\n")
	}
	rb := make([]byte, 0, len(header)+len(m.Body))
	rb = append(rb, header...)
	rb = append(rb, m.Body...)
	return http.ReadResponse(bufio.NewReader(bytes.NewReader(rb)), req)
}
