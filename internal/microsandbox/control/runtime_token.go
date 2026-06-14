package control

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type runtimeEndpointClaims struct {
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
	Purpose   string `json:"purpose"`
	jwt.RegisteredClaims
}

func (s *Server) signRuntimeToken(sandboxID string, port int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := runtimeEndpointClaims{
		SandboxID: sandboxID,
		Port:      port,
		Purpose:   "runtime_endpoint",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sandboxID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.PreviewJWTSecret))
}
