package objectstore

import (
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const controlBodyLimit int64 = 64 << 10

// limitedClient caps bodies before SDK middleware can buffer error/control XML.
// The configured transport still owns header limits, connection buffers and TLS.
type limitedClient struct {
	next         s3.HTTPClient
	successLimit int64
}

func (c limitedClient) Do(request *http.Request) (*http.Response, error) {
	response, err := c.next.Do(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	limit := controlBodyLimit
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		limit = c.successLimit
	}
	response.Body = &limitedBody{next: response.Body, remaining: limit}
	return response, nil
}

type limitedBody struct {
	next      io.ReadCloser
	remaining int64
	exceeded  bool
}

func (b *limitedBody) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if b.exceeded {
		return 0, ErrResponseLimit
	}
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.next.Read(probe[:])
		if n > 0 {
			b.exceeded = true
			return 0, ErrResponseLimit
		}
		return 0, err
	}
	if int64(len(data)) > b.remaining {
		data = data[:int(b.remaining)]
	}
	n, err := b.next.Read(data)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.next.Close() }
