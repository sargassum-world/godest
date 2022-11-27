package actioncable

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// IdentifierChecker returns an error if the provided identifier is invalid.
type IdentifierChecker func(identifier string) error

// Identifier Parsers

// parseChannelName extracts the Action Cable channel name from a JSON-encoded Action Cable
// subscription identifier.
func parseChannelName(identifier string) (channelName string, err error) {
	var i struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal([]byte(identifier), &i); err != nil {
		return "", errors.Wrap(err, "couldn't parse channel name from identifier")
	}
	return i.Channel, nil
}

// parseChannelName extracts the CSRF token from a JSON-encoded Action Cable subscription identifier.
func parseCSRFToken(identifier string) (token string, err error) {
	var i struct {
		Token string `json:"csrfToken"`
	}
	if err := json.Unmarshal([]byte(identifier), &i); err != nil {
		return "", errors.Wrap(err, "couldn't parse csrf token from identifier")
	}
	return i.Token, nil
}

// WithCSRFTokenChecker creates an IdentifierChecker which extracts a CSRF token from the Action
// Cable subscription identifier and checks it using the checker callback function.
func WithCSRFTokenChecker(checker func(token string) error) IdentifierChecker {
	return func(identifier string) error {
		token, err := parseCSRFToken(identifier)
		if err != nil {
			return err
		}
		if err := checker(token); err != nil {
			return errors.Wrap(err, "failed CSRF token check")
		}
		return nil
	}
}
