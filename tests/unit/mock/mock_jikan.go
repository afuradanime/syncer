package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	jikan "github.com/afuradanime/tenrai-go"
)

type JikanResponse struct {
	StatusCode int
	Body       any
	Err        error
}

type Jikan struct {
	testing   testing.TB
	mu        sync.Mutex
	responses map[string][]JikanResponse
	requests  []*http.Request
}

func NewJikan(testing testing.TB) *Jikan {
	testing.Helper()
	mock := &Jikan{testing: testing, responses: make(map[string][]JikanResponse)}
	previous := jikan.Client
	jikan.Client = &http.Client{Transport: mock}
	testing.Cleanup(func() {
		jikan.Client = previous
		mock.mu.Lock()
		defer mock.mu.Unlock()
		for endpoint, responses := range mock.responses {
			if len(responses) != 0 {
				testing.Errorf("unconsumed Jikan responses for %s: %d", endpoint, len(responses))
			}
		}
	})
	return mock
}

func (mock *Jikan) ExpectGET(path string, query url.Values, responses ...JikanResponse) {
	mock.testing.Helper()
	endpoint := (&url.URL{Path: path, RawQuery: query.Encode()}).RequestURI()
	mock.mu.Lock()
	defer mock.mu.Unlock()
	mock.responses[endpoint] = append(mock.responses[endpoint], responses...)
}

func (mock *Jikan) Requests() []*http.Request {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	requests := make([]*http.Request, len(mock.requests))
	for index, request := range mock.requests {
		requests[index] = request.Clone(request.Context())
	}
	return requests
}

func (mock *Jikan) RoundTrip(request *http.Request) (*http.Response, error) {
	mock.mu.Lock()
	mock.requests = append(mock.requests, request.Clone(request.Context()))
	endpoint := (&url.URL{Path: request.URL.Path, RawQuery: request.URL.Query().Encode()}).RequestURI()
	responses := mock.responses[endpoint]
	if request.Method != http.MethodGet || len(responses) == 0 {
		mock.mu.Unlock()
		err := fmt.Errorf("unexpected Jikan request: %s %s", request.Method, request.URL)
		mock.testing.Error(err)
		return nil, err
	}
	response := responses[0]
	mock.responses[endpoint] = responses[1:]
	mock.mu.Unlock()
	if response.Err != nil {
		return nil, response.Err
	}
	var body []byte
	if raw, ok := response.Body.(string); ok {
		body = []byte(raw)
	} else {
		var err error
		body, err = json.Marshal(response.Body)
		if err != nil {
			return nil, fmt.Errorf("encode mock Jikan response: %w", err)
		}
	}
	status := response.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode:    status,
		Status:        strings.TrimSpace(fmt.Sprintf("%d %s", status, http.StatusText(status))),
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}, nil
}
