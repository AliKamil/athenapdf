package http

import (
	"bytes"
	"context"
	"golang.org/x/net/publicsuffix"
	"io"
	"net/http"
	"net/http/cookiejar"

	"github.com/arachnys/athenapdf/pkg/fetcher"
	"github.com/arachnys/athenapdf/pkg/proto"
)

const (
	fetcherName = "http"

	defaultContentType = "text/plain; charset=\"UTF-8\""
)

type HTTPFetcher struct{}

func init() {
	fetcher.Register(fetcherName, &HTTPFetcher{})
}

// TODO: add support for cancellations
func (_ *HTTPFetcher) Fetch(ctx context.Context, target string, opts map[string]*proto.Option) (io.Reader, string, error) {
	// Fetch URL with support for cookies (to handle session-based redirects)
	cookieJarOpts := cookiejar.Options{PublicSuffixList: publicsuffix.List}
	jar, err := cookiejar.New(&cookieJarOpts)
	if err != nil {
		return nil, "", err
	}

	httpReq, err := http.NewRequest("POST", target, nil)
	if err != nil {
		return nil, "", err
	}

	tr := &http.Transport{}
	client := &http.Client{
		Jar:       jar,
		Transport: tr,
	}

	resCh := make(chan struct {
		r   *http.Response
		err error
	}, 1)

	go func() {
		res, err := client.Do(httpReq)
		resCh <- struct {
			r   *http.Response
			err error
		}{res, err}
	}()

	select {
	case <-ctx.Done():
		tr.CancelRequest(httpReq)
		return nil, "", ctx.Err()
	case res := <-resCh:
		if res.err != nil {
			return nil, "", res.err
		}
		if res.r.Body != nil {
			defer res.r.Body.Close()
		}

		contentTypeBuf := bytes.NewBuffer(make([]byte, 0, 512))
		resBuf := bytes.NewBuffer(nil)
		w := io.MultiWriter(contentTypeBuf, resBuf)
		if _, err := io.Copy(w, res.r.Body); err != nil {
			return nil, "", err
		}

		contentType := defaultContentType
		if contentTypeBuf.Len() >= 512 {
			contentType = http.DetectContentType(contentTypeBuf.Bytes())
		} else if headerContentType := res.r.Header.Get("Content-Type"); headerContentType != "" {
			contentType = headerContentType
		}

		return resBuf, contentType, nil
	}
}
