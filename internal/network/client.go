package network

import "net/http"

// Client is the driver that handles outbound requests for the shadows.
type Client struct {
	// Client provides functionality for the struct.
	Client *http.Client
}

// NewClient returns a Client configured with the provided client.
func NewClient(transport *http.Transport) Client {
	client := &http.Client{
		Transport: transport,
	}
	return Client{Client: client}
}
