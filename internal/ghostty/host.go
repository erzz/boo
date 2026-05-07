package ghostty

// Detection helper for the integration-test guardrail. Lives in non-test code
// so it can be unit tested without the integration build tag. Not exported —
// only the test guard uses it.

func detectGhosttyHost(env func(string) string) bool {
	if env("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	if env("GHOSTTY_BIN_DIR") != "" {
		return true
	}
	if env("TERM_PROGRAM") == "ghostty" {
		return true
	}
	return false
}
