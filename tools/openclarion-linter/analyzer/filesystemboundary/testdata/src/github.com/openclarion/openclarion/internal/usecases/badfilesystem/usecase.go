package badfilesystem

import ioutil "io/ioutil"

func write(path string, body []byte) error {
	return ioutil.WriteFile(path, body, 0o600) // want "core domain/usecase code must not access local files directly"
}

func list(path string) error {
	_, err := ioutil.ReadDir(path) // want "core domain/usecase code must not access local files directly"
	return err
}
