package authn

import (
	"fmt"
	"math"

	"github.com/alexedwards/argon2id"
	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/env"
)

const envPrefix = "AUTHN_"

type Config struct {
	NoAuth            bool
	Argon2idParams    argon2id.Params
	AdminUsername     string
	AdminPasswordHash string
}

func GetConfig() (c Config, err error) {
	c.NoAuth, err = getNoAuth()
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make authentication config")
	}

	c.Argon2idParams, err = getArgon2idParams()
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make password hashing config")
	}

	c.AdminUsername = getAdminUsername()

	c.AdminPasswordHash, err = getAdminPasswordHash(c.Argon2idParams, c.NoAuth)
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make admin password hash config")
	}

	return c, nil
}

func getNoAuth() (bool, error) {
	return env.GetBool(envPrefix + "NOAUTH")
}

func getArgon2idParams() (argon2id.Params, error) {
	var defaultMemorySize uint64 = 64 // default: 64 MiB
	memorySize, err := env.GetUint64(envPrefix+"ARGON2ID_M", defaultMemorySize)
	if err != nil {
		return argon2id.Params{}, errors.Wrap(err, "couldn't make memorySize config")
	}
	memorySize *= 1024
	if memorySize > math.MaxUint32 {
		return argon2id.Params{}, errors.Errorf("%sARGON2ID_M %d is too large!", envPrefix, memorySize)
	}

	var defaultIterations uint64 = 1 // default: 1 iteration over the memory
	iterations, err := env.GetUint64(envPrefix+"ARGON2ID_T", defaultIterations)
	if err != nil {
		return argon2id.Params{}, errors.Wrap(err, "couldn't make iterations config")
	}
	if iterations > math.MaxUint32 {
		return argon2id.Params{}, errors.Errorf("%sARGON2ID_T %d is too large!", envPrefix, iterations)
	}

	var defaultParallelism uint64 = 2 // default: 2 threads/lanes
	parallelism, err := env.GetUint64(envPrefix+"ARGON2ID_P", defaultParallelism)
	if err != nil {
		return argon2id.Params{}, errors.Wrap(err, "couldn't make parallelism config")
	}
	if parallelism > math.MaxUint8 {
		return argon2id.Params{}, errors.Errorf("%sARGON2ID_P %d is too large!", envPrefix, parallelism)
	}

	var defaultSaltLength uint32 = 16 // default: 16 bytes
	var defaultKeyLength uint32 = 32  // default: 32 bytes
	return argon2id.Params{
		Memory:      uint32(memorySize),
		Iterations:  uint32(iterations),
		Parallelism: uint8(parallelism),
		SaltLength:  defaultSaltLength,
		KeyLength:   defaultKeyLength,
	}, nil
}

func getAdminUsername() string {
	return env.GetString(envPrefix+"ADMIN_USERNAME", "admin")
}

func getAdminPasswordHash(argon2idParams argon2id.Params, noAuth bool) (hash string, err error) {
	hash = env.GetString(envPrefix+"ADMIN_PW_HASH", "")
	if len(hash) == 0 && !noAuth {
		password := env.GetString(envPrefix+"ADMIN_PW", "")
		if len(password) == 0 {
			return "", errors.Errorf(
				"must provide a password for the admin account with %sADMIN_PW", envPrefix,
			)
		}

		hash, err = argon2id.CreateHash(password, &argon2idParams)
		if err != nil {
			return "", err
		}
		fmt.Printf(
			"Record this admin password hash for future use as %sADMIN_PW_HASH "+
				"(use single-quotes from shell to avoid string substitution with dollar-signs): %s\n",
			envPrefix, hash,
		)
	}

	return hash, nil
}
