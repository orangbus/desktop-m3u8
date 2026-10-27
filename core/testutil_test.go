package core

import "os"

func writeFileForTest(p string) error {
	return os.WriteFile(p, []byte("x"), 0o644)
}
