package httpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONBodyError_Error(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var e JSONBodyError

		got := e.Error()

		assert.Equal(t, "http body: null", got)
	})

	t.Run("error", func(t *testing.T) {
		e := JSONBodyError{
			"foo": "bar",
		}

		got := e.Error()

		assert.Equal(t, `http body: {"foo":"bar"}`, got)
	})
}

func TestBytesBodyError_Error(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var e BytesBodyError

		got := e.Error()

		assert.Equal(t, "http body: ", got)
	})

	t.Run("error", func(t *testing.T) {
		e := BytesBodyError("hello world")

		got := e.Error()

		assert.Equal(t, `http body: hello world`, got)
	})
}
