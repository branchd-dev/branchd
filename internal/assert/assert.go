package assert

import (
	"fmt"
)

func Length(value string, expected int) {
	if len(value) != expected {
		msg := fmt.Sprintf("assert.Length expected %d actual %d", expected, len(value))
		panic(msg)
	}
}
