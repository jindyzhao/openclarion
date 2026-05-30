package badglobalstate

var severityRank = map[string]int{ // want "core domain/usecase code must not keep mutable package-level collection state"
	"info": 1,
}

var knownStates = []string{"new"} // want "core domain/usecase code must not keep mutable package-level collection state"

var hiddenStates any = []string{"hidden"} // want "core domain/usecase code must not keep mutable package-level collection state"

var statePtr = new([]string) // want "core domain/usecase code must not keep mutable package-level collection state"
