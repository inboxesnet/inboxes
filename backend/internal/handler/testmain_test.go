package handler

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests must not call the external HIBP API.
	os.Setenv("HIBP_CHECK_DISABLED", "true")
	os.Exit(m.Run())
}
