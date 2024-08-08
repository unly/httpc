# httpc

Simple wrapping of the existing `http.Client` with some additional functionality.

## How to use

### stdlib compatibility

As the client extends a `*http.Client` you can use the existing functionality as defined in the stdlib.

```go
client := httpc.New()

resp, err := client.Get("example.com")
... // handle the response and error

resp, err := client.Do(req)
...

underlyingClient := client.Unwrap() // of type *http.Client
// pass this to calls that require type *http.Client
// still contains all layers of http.RoundTripper
```

### Extensions

Beyond the existing functions of the stdlib http client there are few convenience wrappers in place to help reduce repetitive coding tasks.
All outgoing calls `DoReq()`, `JSON()` and `Stream()` close the response's body and replace it with a [NopCloser](https://pkg.go.dev/io#NopCloser).

```go
client := httpc.New(WithTimeout(10 * time.Second)) // optional options can be passed to the initial setup
h := http.Header{}
h.Set("key", "value")
client.AddOptions(WithHeaders(h)) // further options can be added

resp, err := client.DoReq(req, WithStatusCode(http.StatusOk)) // call like Do() with additional check of the status code
...

type Person struct {
	FirstName string `json:"firstName"`
	LastName string `json:"lastName"`
}

var p Person
resp, err := client.JSON(req, &p, WithStatusCode(http.StatusOk)) // decodes the response to the given pointer
...

f, _ := os.OpenFile("data.out", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
written, err := client.Stream(req, f) // stream the response to a file
...
```

### Error Handling

Only additional functionality calls do support error handling.
For HTTP status codes above `400` the error handler is called.
By default, the response body is read as is and be added to the error message.
See the examples below to add custom error types.

```go
client := httpc.New(WithJSONError())

_, err := client.DoReq(req) // if fails, attempts to parse response body as arbitrary JSON
// err should be of type JSONErrorBody map[string]any
// can be used to access error details

type ApiError struct {
	Code int `json:"code"`
	Msg string `json:"msg"`
}

client := httpc.New(WithCustomJSONError[ApiError]())

_, err := client.DoReq(req) // if fails, attempts to parse response body as given error
// err should be of type ApiError
```
