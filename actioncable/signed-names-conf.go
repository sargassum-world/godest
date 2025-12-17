package actioncable

import (
	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/env"
)

const envPrefix = "ACTIONCABLE_"

type SignerConfig struct {
	KeyFile string
	HashKey []byte
}

func GetSignerConfig() (c SignerConfig, err error) {
	const defaultKeyfilePath = "/tmp/action-cable.key"
	c.KeyFile = env.GetString(envPrefix+"HASH_KEYFILE", defaultKeyfilePath)

	const hashKeySize = 32
	if c.HashKey, err = env.GetKeyWithFile(envPrefix+"HASH_KEY", c.KeyFile, hashKeySize); err != nil {
		return c, errors.Wrapf(err, "couldn't get Action Cable key")
	}

	return c, nil
}
