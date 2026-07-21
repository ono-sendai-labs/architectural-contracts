package core

// Helper lives in an embedded library but the same package.
func Helper() string { return extraHelper() }
