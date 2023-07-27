package mockserver

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/stringutil"
)

//go:embed testdata/projects.json
var ProjectsJSON string

//go:embed testdata/error_details.json
var ErrorDetailsJSON string

type MockServer struct {
	Address  string
	Handlers map[string]http.HandlerFunc
}

func New(t *testing.T) *MockServer {
	return &MockServer{
		Handlers: map[string]http.HandlerFunc{
			"/": handleDefault(t),
		},
	}
}

func (server *MockServer) Start(t *testing.T) {
	mux := http.NewServeMux()
	for path, handler := range server.Handlers {
		mux.Handle(path, handler)
	}

	listener, err := net.Listen("tcp4", ":0")
	require.NoError(t, err)

	server.Address = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err = http.Serve(listener, mux)
		require.NoError(t, err)
	}()
}

func ReturnResponse(t *testing.T, responseString string) http.HandlerFunc {
	return CheckBodyAndReturnResponse(t, responseString, nil, nil)
}

func CheckBodyAndReturnResponse(t *testing.T, responseString string, expected []string, unexpected []string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		for _, expectedString := range expected {
			require.Contains(t, string(body), expectedString)
		}
		for _, unexpectedString := range unexpected {
			require.NotContains(t, string(body), unexpectedString)
		}
		_, err = io.WriteString(w, responseString)
		require.NoError(t, err)
	}
}

func handleDefault(t *testing.T) http.HandlerFunc {
	return func(_ http.ResponseWriter, req *http.Request) {
		require.Fail(t, "Unexpected request", stringutil.PrettyString(req))
	}
}
