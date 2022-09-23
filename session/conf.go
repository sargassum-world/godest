package session

import (
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/env"
)

const envPrefix = "SESSIONS_"

type Timeouts struct {
	Absolute time.Duration
	// TODO: add idle timeout
	// TODO: add renewal timeout, if we can implement session renewal
}

type CSRFOptions struct {
	HeaderName string
	FieldName  string
}

type Config struct {
	AuthKey       []byte
	EncryptionKey []byte
	Timeouts      Timeouts
	CookieOptions sessions.Options
	CookieName    string
	CSRFOptions   CSRFOptions
}

func GetConfig() (c Config, err error) {
	// TODO: support key rotation
	const authKeySize = 32
	c.AuthKey, err = env.GetKey(envPrefix+"AUTH_KEY", authKeySize)
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make session auth key config")
	}

	const encryptionKeySize = 32
	c.EncryptionKey, err = env.GetKey(envPrefix+"ENCRYPTION_KEY", encryptionKeySize)
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make session encryption key config")
	}

	c.Timeouts, err = getTimeouts()
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make session timeouts config")
	}

	// TODO: when we implement idle timeout, pass that instead of absolute timeout
	c.CookieOptions, err = getCookieOptions()
	if err != nil {
		return Config{}, errors.Wrap(err, "couldn't make cookie options config")
	}

	if c.CookieOptions.Secure {
		// The __Host- prefix requires Secure, HTTPS, no Domain, and path "/"
		c.CookieName = "__Host-Session"
	} else {
		c.CookieName = "session"
	}

	c.CSRFOptions = getCSRFOptions()
	return c, nil
}

func getTimeouts() (t Timeouts, err error) {
	const defaultAbsolute = 60 * 24 * 7 * 24 // default: 1 week
	rawAbsolute, err := env.GetInt64(envPrefix+"TIMEOUTS_ABSOLUTE", defaultAbsolute)
	if err != nil {
		return Timeouts{}, errors.Wrap(err, "couldn't make absolute timeout config")
	}
	t.Absolute = time.Duration(rawAbsolute) * time.Minute
	return t, nil
}

func getCookieOptions() (o sessions.Options, err error) {
	noHTTPSOnly, err := env.GetBool(envPrefix + "COOKIE_NOHTTPSONLY")
	if err != nil {
		return sessions.Options{}, errors.Wrap(err, "couldn't make HTTPS-only config")
	}

	return sessions.Options{
		Path:     "/",
		Domain:   "",
		MaxAge:   0,
		Secure:   !noHTTPSOnly,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

func getCSRFOptions() (o CSRFOptions) {
	o.HeaderName = env.GetString(envPrefix+"CSRF_HEADERNAME", "X-CSRF-Token")
	o.FieldName = env.GetString(envPrefix+"CSRF_FIELDNAME", "csrf-token")
	return o
}
