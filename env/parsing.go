// Package env contains code for handling environment variables
package env

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

func GetBool(varName string) (bool, error) {
	value := os.Getenv(varName)
	if len(value) == 0 {
		return false, nil
	}

	switch value {
	case "TRUE", "true", "True":
		return true, nil
	case "FALSE", "false", "False":
		return false, nil
	}

	return false, errors.Errorf(
		"unknown value %s for boolean environment variable %s", value, varName,
	)
}

func GetUint64(varName string, defaultValue uint64) (uint64, error) {
	value := os.Getenv(varName)
	if len(value) == 0 {
		return defaultValue, nil
	}

	const (
		base  = 10
		width = 32 // bits
	)
	parsed, err := strconv.ParseUint(value, base, width)
	if err != nil {
		return 0, errors.Wrapf(
			err, "unparsable value %s for uint64 environment variable %s", value, varName,
		)
	}

	return parsed, nil
}

func GetInt64(varName string, defaultValue int64) (int64, error) {
	value := os.Getenv(varName)
	if len(value) == 0 {
		return defaultValue, nil
	}

	const (
		base  = 10
		width = 64 // bits
	)
	parsed, err := strconv.ParseInt(value, base, width)
	if err != nil {
		return 0, errors.Wrapf(
			err, "unparsable value %s for int64 environment variable %s", value, varName,
		)
	}

	return parsed, nil
}

func GetFloat32(varName string, defaultValue float32) (float32, error) {
	value := os.Getenv(varName)
	if len(value) == 0 {
		return defaultValue, nil
	}

	const width = 32 // bits
	parsed, err := strconv.ParseFloat(value, width)
	if err != nil {
		return 0, errors.Wrapf(
			err, "unparsable value %s for float32 environment variable %s", value, varName,
		)
	}

	return float32(parsed), nil
}

func GetString(varName string, defaultValue string) string {
	value := os.Getenv(varName)
	if len(value) == 0 {
		return defaultValue
	}

	return value
}

func GetBase64(varName string) ([]byte, error) {
	rawValue := os.Getenv(varName)
	if len(rawValue) == 0 {
		return nil, nil
	}

	return base64.StdEncoding.DecodeString(rawValue)
}

func GetURL(varName string, defaultValue string) (*url.URL, error) {
	value := os.Getenv(varName)
	if len(value) == 0 {
		value = defaultValue
	}

	return url.Parse(value)
}

func GetURLOrigin(varName, defaultValue, defaultScheme string) (*url.URL, error) {
	url, err := GetURL(varName, defaultValue)
	if err != nil {
		return nil, errors.Wrapf(
			err, "unparsable value %s for URL environment variable %s", os.Getenv(varName), varName,
		)
	}

	if len(url.Scheme) == 0 {
		url.Scheme = defaultScheme
	}
	url.Path = ""
	url.User = nil
	url.RawQuery = ""
	url.Fragment = ""

	return url, nil
}

// GenerateRandomKey creates a random key with the given length in bytes.
// On failure, returns nil.
//
// Note that keys created using `GenerateRandomKey()` are not automatically
// persisted. New keys will be created when the application is restarted, and
// previously issued cookies will not be able to be decoded.
//
// Callers should explicitly check for the possibility of a nil return, treat
// it as a failure of the system random number generator, and not continue.
func GenerateRandomKey(length int) []byte {
	// Note: copied from github.com/gorilla/securecookie
	k := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil
	}
	return k
}

func GetKey(varName string, length int) ([]byte, error) {
	key, err := GetBase64(varName)
	if err != nil {
		return nil, err
	}

	if key == nil {
		key = GenerateRandomKey(length)
		if key == nil {
			return nil, errors.New("unable to generate a random key")
		}
		// TODO: print to the logger instead?
		fmt.Printf(
			"Record this key for future use as %s: %s\n",
			varName, base64.StdEncoding.EncodeToString(key),
		)
	}
	return key, nil
}

func GetKeyWithFile(varName, filePath string, genLength int) ([]byte, error) {
	key, err := GetBase64(varName)
	if err == nil && len(key) > 0 {
		return key, nil
	}

	if filePath != "" {
		if key, err = GetKeyFromFile(filePath); err != nil {
			fmt.Printf("Warning: couldn't load key for %s from %s: %s\n", varName, filePath, err)
		}
	}
	if len(key) > 0 {
		fmt.Printf("Using key for %s from %s\n", varName, filePath)
		return key, nil
	}

	key = GenerateRandomKey(genLength)
	if err = SaveKeyToFile(filePath, key); err != nil {
		fmt.Printf(
			"Warning: couldn't save newly-generated key for %s to %s: %e\n", varName, filePath, err,
		)
		fmt.Printf(
			"Record this key for future use as %s, preferably in %s: %s\n",
			varName, filePath, base64.StdEncoding.EncodeToString(key),
		)
	}
	fmt.Printf("Generated and saved random key for %s to %s!", varName, filePath)
	return key, nil
}

func GetKeyFromFile(path string) (key []byte, err error) {
	path = filepath.Clean(path)
	rawFile, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't read keyfile %s", path)
	}
	if key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawFile))); err != nil {
		return nil, errors.Wrapf(
			err, "couldn't parse key of length %d from file %s", len(string(rawFile)), path,
		)
	}
	return key, nil
}

func SaveKeyToFile(path string, key []byte) error {
	const dirPerm = 0o755 // owner rwx, group rx, public rx
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrapf(err, "couldn't create parent directory for file %s", path)
	}
	const filePerm = 0o600 // owner rw, group none, public none
	path = filepath.Clean(path)
	if err := os.WriteFile(
		path, []byte(base64.StdEncoding.EncodeToString(key)), filePerm,
	); err != nil {
		return errors.Wrapf(err, "couldn't save key to %s", path)
	}
	return nil
}
