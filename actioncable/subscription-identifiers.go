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
