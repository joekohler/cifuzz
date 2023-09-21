package mockserver

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/stringutil"
)

const ValidToken = "valid-token"
const InvalidToken = "invalid-token"

//go:embed testdata/projects.json
var ProjectsJSON string

//go:embed testdata/error_details.json
var ErrorDetailsJSON string

//go:embed testdata/remote_findings.json
var RemoteFindingsJSON string

//go:embed testdata/container_remote_run.json
var ContainerRemoteRunResponse string

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

func (server *MockServer) StartForContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode (requires docker)")
	}
	mux := http.NewServeMux()
	for path, handler := range server.Handlers {
		mux.Handle(path, handler)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	ctx := context.Background()

	networkID := "bridge"
	// On Windows, the default network is 'nat'.
	if runtime.GOOS == "windows" {
		networkID = "nat"
	}

	networkInspect, err := cli.NetworkInspect(ctx, networkID, types.NetworkInspectOptions{
		Verbose: false,
	})
	require.NoError(t, err)

	addr := networkInspect.IPAM.Config[0].Gateway

	listener, err := net.Listen("tcp4", ":0")
	require.NoError(t, err)

	server.Address = fmt.Sprintf("http://%s:%d", addr, listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err = http.Serve(listener, mux)
		require.NoError(t, err)
	}()
}

func ReturnResponse(t *testing.T, responseString string) http.HandlerFunc {
	return CheckBodyAndReturnResponse(t, responseString, nil, nil)
}

func ReturnResponseIfValidToken(t *testing.T, responseString string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "Bearer "+ValidToken {
			CheckBodyAndReturnResponse(t, responseString, nil, nil)(w, req)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
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
