package actioncable

import (
	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/env"
)

const envPrefix = "ACTIONCABLE_"

type SignerConfig struct {
	HashKey []byte
}

func GetSignerConfig() (c SignerConfig, err error) {
	const hashKeySize = 32
	c.HashKey, err = env.GetKey(envPrefix+"HASH_KEY", hashKeySize)
	if err != nil {
		return SignerConfig{}, errors.Wrap(err, "couldn't make action cable name signer config")
	}

	return c, nil
}
