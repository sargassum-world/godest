package turbostreams

import (
	"net/http"
	"strings"
)

// ContentType is the MIME type of form submissions for Turbo Stream responses.
const ContentType = "text/vnd.turbo-stream.html"

// Accepted checks the [http.Header]'s ContentType to determine whether to accept it as a Turbo
// Streams form submission.
func Accepted(h http.Header) bool {
	for _, a := range strings.Split(h.Get("Accept"), ",") {
		if strings.TrimSpace(a) == ContentType {
			return true
		}
	}
	return false
}
