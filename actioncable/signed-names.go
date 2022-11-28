package actioncable

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"

	"github.com/pkg/errors"
)

// Signer creates and verifies subscription identifier names with an HMAC.
type Signer struct {
	Config SignerConfig
}

// NewSigner creates a new instance of [Signer].
func NewSigner(config SignerConfig) Signer {
	return Signer{
		Config: config,
	}
}

// Check parses the subscription identifier's name field and checks if it matches the HMAC in the
// subscription identiier's hash field. Note that these fields are not part of the Action Cable
// specification; rather, they're conventions inherited from the Rails implementation of integration
// between Turbo Streams and Action Cable.
func (s Signer) Check(identifier string) error {
	name, err := s.parseIdentifier(identifier)
	if err != nil {
		return err
	}
	if !s.validate(name) {
		return errors.Errorf("signed stream/subchannel name %s failed HMAC check", name.Name)
	}
	return nil
}

// signedName is a pair of a name and its HMAC.
type signedName struct {
	Name string
	Hash []byte
}

// validate checks for consistency between the name and HMAC in a signedName.
func (s Signer) validate(n signedName) bool {
	return hmac.Equal(n.Hash, s.hash(n.Name))
}

// parseIdentifier parses the JSON-encoded subscription identifier into a signedName.
func (s Signer) parseIdentifier(identifier string) (parsed signedName, err error) {
	var params struct {
		Name string `json:"name"`
		Hash string `json:"integrity"`
	}
	if err = json.Unmarshal([]byte(identifier), &params); err != nil {
		return signedName{}, errors.Wrap(err, "couldn't parse identifier for params")
	}
	parsed.Name = params.Name
	parsed.Hash, err = base64.StdEncoding.DecodeString(params.Hash)
	if err != nil {
		return signedName{}, errors.Wrap(err, "couldn't base64-decode stream/subchannel name hash")
	}
	return parsed, nil
}

// hash computes the HMAC of the name.
func (s Signer) hash(name string) []byte {
	h := hmac.New(sha512.New, s.Config.HashKey)
	h.Write([]byte(name))
	return h.Sum(nil)
}

// Sign computes the base64-encoded string of the HMAC of the name.
func (s Signer) Sign(name string) (hash string) {
	return base64.StdEncoding.EncodeToString(s.hash(name))
}
