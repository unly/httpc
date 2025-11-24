package httpc

import (
	"encoding/json"
	"fmt"
)

// JSONBodyError arbitrary JSON representation of an HTTP body.
type JSONBodyError map[string]any

func (e JSONBodyError) Error() string {
	encoded, _ := json.Marshal(e)
	return fmt.Sprintf("http body: %s", encoded)
}

// BytesBodyError raw []byte representation of an HTTP body.
type BytesBodyError []byte

func (e BytesBodyError) Error() string {
	return fmt.Sprintf("http body: %s", string(e))
}
