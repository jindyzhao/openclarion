package badglobalstate

type Token []byte

var token = Token("mutable") // want "core domain/usecase code must not keep mutable package-level collection state"

var queue = make(chan string) // want "core domain/usecase code must not keep mutable package-level collection state"
