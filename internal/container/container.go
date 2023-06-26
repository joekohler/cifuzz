package container

import (
	"context"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/pkg/log"
)

func Create(fuzzTest string) (string, error) {
	cli, err := getDockerClient()
	if err != nil {
		return "", err
	}

	containerConfig := &container.Config{
		Image: "cifuzz",
		Tty:   false,
		Cmd:   []string{"cifuzz", "execute", fuzzTest},
	}

	if viper.GetBool("verbose") {
		containerConfig.Cmd = append(containerConfig.Cmd, "-v")
	}

	ctx := context.Background()
	cont, err := cli.ContainerCreate(
		ctx,
		containerConfig,
		nil,
		nil,
		&v1.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		"", // TODO: should the container have a name?
	)
	if err != nil {
		return "", errors.WithStack(err)
	}

	log.Debugf("Created fuzz container %s based on image %s", cont.ID, containerConfig.Image)
	return cont.ID, nil
}

func Start(id string) error {
	cli, err := getDockerClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	err = cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	log.Debugf("started container %s", id)

	// Continuously print the container's stdout and stderr to the host's
	// stdout and stderr.
	go func() {
		out, err := cli.ContainerLogs(ctx, id, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			log.Errorf(err, "error getting container logs: %s", err.Error())
			return
		}
		defer func() {
			err := out.Close()
			if err != nil {
				log.Errorf(err, "error closing container logs: %s", err.Error())
			}
		}()

		_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
		if err != nil {
			log.Errorf(err, "error copying container logs: %s", err.Error())
			return
		}
	}()

	statusCh, errCh := cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case err = <-errCh:
		if err != nil {
			return errors.WithStack(err)
		}
	case <-statusCh:
	}

	return err
}

func Stop(id string) error {
	cli, err := getDockerClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	err = cli.ContainerStop(ctx, id, container.StopOptions{})
	return errors.WithStack(err)
}
