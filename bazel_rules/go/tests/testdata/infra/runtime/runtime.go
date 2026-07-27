package runtime

import "os"

func InjectedReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
