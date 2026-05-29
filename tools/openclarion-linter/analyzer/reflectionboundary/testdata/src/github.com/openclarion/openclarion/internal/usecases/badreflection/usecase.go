package badreflection

import (
	"reflect" // want "production code must not import reflect directly"
	"unsafe"  // want "production code must not import unsafe directly"
)

func inspect(value any) uintptr {
	return uintptr(unsafe.Sizeof(value)) + uintptr(reflect.TypeOf(value).Size())
}
