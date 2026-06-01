package badflag

import "flag"

func parse() {
	_ = flag.String("config", "", "path") // want "core domain/usecase code must not parse process flags directly"
	flag.Parse()                          // want "core domain/usecase code must not parse process flags directly"
}
