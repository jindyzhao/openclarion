package badfilesystem

import "os"

func read(path string) ([]byte, error) {
	return os.ReadFile(path) // want "core domain/usecase code must not access local files directly"
}

func metadata(path string) error {
	_, err := os.Stat(path) // want "core domain/usecase code must not access local files directly"
	return err
}

func list(path string) error {
	_, err := os.ReadDir(path) // want "core domain/usecase code must not access local files directly"
	return err
}

func tempDir(parent string) error {
	_, err := os.MkdirTemp(parent, "openclarion-*") // want "core domain/usecase code must not access local files directly"
	return err
}

func link(target, link string) error {
	return os.Symlink(target, link) // want "core domain/usecase code must not access local files directly"
}

func localFS(root string) {
	_ = os.DirFS(root) // want "core domain/usecase code must not access local files directly"
}
