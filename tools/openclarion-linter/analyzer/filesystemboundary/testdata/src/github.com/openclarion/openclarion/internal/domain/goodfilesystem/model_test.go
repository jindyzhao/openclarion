package goodfilesystem

import "os"

func fixture(path string) ([]byte, error) {
	return os.ReadFile(path)
}
