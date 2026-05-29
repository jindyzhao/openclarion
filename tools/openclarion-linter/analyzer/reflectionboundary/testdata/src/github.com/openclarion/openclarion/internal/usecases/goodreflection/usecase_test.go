package goodreflection

import "reflect"

func same(got, want any) bool {
	return reflect.DeepEqual(got, want)
}
