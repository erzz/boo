package ghostty

// detectGhosttyHost reports whether the process is running inside Ghostty.
// Lives in non-test code so it can be unit-tested without the integration tag.
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
