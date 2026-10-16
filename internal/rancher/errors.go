package rancher

import "errors"

// ErrTokenInvalid indicates the Rancher API token is invalid or expired.
var ErrTokenInvalid = errors.New("rancher token invalid or expired")

// ErrLoginFailed indicates that a Rancher auth provider rejected the username or password.
var ErrLoginFailed = errors.New("rancher auth provider login failed: invalid username or password")
