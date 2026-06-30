package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirebaseSessionRoundTrip(t *testing.T) {
	encoded := EncodeFirebaseSession("signed.access.jwt", "signed.refresh.jwt")

	access, refresh, err := DecodeFirebaseSession(encoded)
	require.NoError(t, err)
	assert.Equal(t, "signed.access.jwt", access)
	assert.Equal(t, "signed.refresh.jwt", refresh)
}

func TestFirebaseSessionRejectsInvalidValues(t *testing.T) {
	_, _, err := DecodeFirebaseSession("not-base64!")
	assert.Error(t, err)

	incomplete := EncodeFirebaseSession("access", "")
	_, _, err = DecodeFirebaseSession(incomplete)
	assert.Error(t, err)
}
