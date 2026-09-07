package stdlibmap

import "testing"

func TestNativeLoaderFipsTest(t *testing.T) {
	l := &NativeLoader{}
	out, err := l.Load([]string{"crypto/internal/fips140test"})
	t.Logf("err=%v out=%v", err, out)
}
