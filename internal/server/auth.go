package server

import "net/http"

// User represents an authenticated caller.
type User struct {
	Email   string
	IsAdmin bool
}

// Authenticator validates requests.
type Authenticator interface {
	Authenticate(r *http.Request) (*User, error)
}

// NoopAuthenticator allows all requests (development only).
type NoopAuthenticator struct{}

func (NoopAuthenticator) Authenticate(r *http.Request) (*User, error) {
	return &User{Email: "anonymous", IsAdmin: true}, nil
}
