package apps

import "net/http"

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return ""
}
