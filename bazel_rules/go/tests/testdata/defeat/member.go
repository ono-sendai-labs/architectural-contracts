package defeat

import "strings"

// Name keeps an ordinary standard-library reference in the fixture so the
// checked action also consumes target-configured export data.
func Name() string { return strings.TrimSpace("defeat") }
