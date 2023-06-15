package container

import (
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

var dockerClient *client.Client

// getDockerClient returns a docker client and will also handle its closing. It will take configuration options in the future.
func getDockerClient() (*client.Client, error) {
	if dockerClient != nil {
		return dockerClient, nil
	}
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer dockerClient.Close()
	return dockerClient, nil
}
