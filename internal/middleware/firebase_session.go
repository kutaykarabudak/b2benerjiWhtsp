package middleware

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// FirebaseSessionCookieName is the only cookie name Firebase Hosting forwards
// to a rewritten Cloud Run service.
const FirebaseSessionCookieName = "__session"

type firebaseSession struct {
	Access  string `json:"a"`
	Refresh string `json:"r"`
}

// EncodeFirebaseSession bundles the existing signed JWTs into the single
// cookie that Firebase Hosting permits. The JWT signatures remain the source
// of authenticity; this wrapper only provides transport encoding.
func EncodeFirebaseSession(accessToken, refreshToken string) string {
	payload, _ := json.Marshal(firebaseSession{Access: accessToken, Refresh: refreshToken})
	return base64.RawURLEncoding.EncodeToString(payload)
}

// DecodeFirebaseSession extracts the two signed JWTs from a Firebase session
// cookie. Each JWT is still validated independently by its normal consumer.
func DecodeFirebaseSession(value string) (accessToken, refreshToken string, err error) {
	if value == "" {
		return "", "", errors.New("empty firebase session")
	}

	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", "", errors.New("invalid firebase session encoding")
	}

	var session firebaseSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return "", "", errors.New("invalid firebase session payload")
	}
	if session.Access == "" || session.Refresh == "" {
		return "", "", errors.New("incomplete firebase session")
	}

	return session.Access, session.Refresh, nil
}
