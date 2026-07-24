package githubauth

import (
	"context"
	"errors"
	"strings"
)

// Environment variable names used by GitHub authentication helpers.
const (
	EnvGitHubCLIToken = "GH_TOKEN"
	EnvGitHubToken    = "GITHUB_TOKEN"
	EnvGitHubAPIToken = "GITHUB_API_TOKEN"
)

var tokenPreference = []string{
	EnvGitHubCLIToken,
	EnvGitHubToken,
	EnvGitHubAPIToken,
}

const tokenMissingMessage = "missing GitHub authentication token; configure github.credential in config.yml"

type credentialContextKey struct{}

// WithCredential attaches the concrete config-resolved GitHub credential to an execution context.
func WithCredential(parentContext context.Context, credential string) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	return context.WithValue(parentContext, credentialContextKey{}, strings.TrimSpace(credential))
}

// ResolveToken returns the first concrete GitHub token supplied by the command or configuration context.
func ResolveToken(executionContext context.Context, environment map[string]string) (string, bool) {
	for _, key := range tokenPreference {
		if value, ok := lookup(environment, key); ok {
			return value, true
		}
	}
	if executionContext != nil {
		if credential, available := executionContext.Value(credentialContextKey{}).(string); available {
			credential = strings.TrimSpace(credential)
			if credential != "" {
				return credential, true
			}
		}
	}
	return "", false
}

func lookup(environment map[string]string, key string) (string, bool) {
	if environment == nil {
		return "", false
	}
	value, exists := environment[key]
	if !exists {
		return "", false
	}
	value = strings.TrimSpace(value)
	if len(value) == 0 {
		return "", false
	}
	return value, true
}

// TokenRequirement describes the token validation strategy for GitHub commands.
type TokenRequirement int

const (
	TokenRequired TokenRequirement = iota
	TokenOptional
)

// MissingTokenError surfaces missing GitHub authentication tokens.
type MissingTokenError struct {
	Operation string
	Critical  bool
}

// Error returns the canonical missing-token message.
func (err MissingTokenError) Error() string {
	return tokenMissingMessage
}

// Is enables errors.Is checks against MissingTokenError sentinels.
func (err MissingTokenError) Is(target error) bool {
	switch typed := target.(type) {
	case MissingTokenError:
		return err.Critical == typed.Critical
	case *MissingTokenError:
		return err.Critical == typed.Critical
	default:
		return false
	}
}

// Critical reports whether the missing token should be treated as fatal.
func (err MissingTokenError) CriticalRequirement() bool {
	return err.Critical
}

// ErrTokenMissing denotes a critical missing token.
var ErrTokenMissing = MissingTokenError{Critical: true}

// ErrTokenMissingOptional denotes a missing token for an optional GitHub operation.
var ErrTokenMissingOptional = MissingTokenError{Critical: false}

// NewMissingTokenError constructs a MissingTokenError for the given operation.
func NewMissingTokenError(operation string, critical bool) MissingTokenError {
	return MissingTokenError{Operation: operation, Critical: critical}
}

// IsMissingTokenError returns the underlying MissingTokenError when present.
func IsMissingTokenError(err error) (MissingTokenError, bool) {
	var missing MissingTokenError
	if errors.As(err, &missing) {
		return missing, true
	}
	return MissingTokenError{}, false
}
