package network

import "net/http"

// ShadowClient is the driver that handles outbound requests for the shadows.
type ShadowClient struct {
	// client provides functionality for the struct.
	client *http.Client
}

// NewShadowClient returns a Client used for shadowing requests.
func NewShadowClient() *ShadowClient {
	client := &http.Client{
		Transport: newShadowTransport(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &ShadowClient{client: client}
}

// Do sends an HTTP request and returns an HTTP response.
func (c *ShadowClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// CloseIdleConnections closes any connections on its Transport which were previously connected from previous requests but are now sitting idle in a "keep-alive" state.
func (c *ShadowClient) CloseIdleConnections() {
	c.client.CloseIdleConnections()
}
