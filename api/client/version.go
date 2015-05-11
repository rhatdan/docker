package client

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/autogen/dockerversion"
	flag "github.com/docker/docker/pkg/mflag"
)

// CmdVersion shows Docker version information.
//
// Available version information is shown for: client Docker version, client API version, client Go version, client Git commit, client OS/Arch, server Docker version, server API version, server Go version, server Git commit, and server OS/Arch.
//
// Usage: docker version
func (cli *DockerCli) CmdVersion(args ...string) error {
	cmd := cli.Subcmd("version", "", "Show the Docker version information.", true)
	cmd.Require(flag.Exact, 0)

	cmd.ParseFlags(args, false)

	if dockerversion.VERSION != "" {
		fmt.Fprintf(cli.out, "Client version: %s\n", dockerversion.VERSION)
	}
	fmt.Fprintf(cli.out, "Client API version: %s\n", api.APIVERSION)
	if output, err := exec.Command("rpm", "-q", "docker").Output(); err == nil {
		fmt.Fprintf(cli.out, "Client package version: %s", output)
	}
	fmt.Fprintf(cli.out, "Go version (client): %s\n", runtime.Version())
	if dockerversion.GITCOMMIT != "" {
		fmt.Fprintf(cli.out, "Git commit (client): %s\n", dockerversion.GITCOMMIT)
	}
	fmt.Fprintf(cli.out, "OS/Arch (client): %s/%s\n", runtime.GOOS, runtime.GOARCH)

	stream, _, err := cli.call("GET", "/version", nil, nil)
	if err != nil {
		return err
	}

	var v types.Version
	if err := json.NewDecoder(stream).Decode(&v); err != nil {
		logrus.Errorf("Error reading remote version: %s", err)
		return err
	}

	fmt.Fprintf(cli.out, "Server version: %s\n", v.Version)
	if v.ApiVersion != "" {
		fmt.Fprintf(cli.out, "Server API version: %s\n", v.ApiVersion)
	}
	fmt.Fprintf(cli.out, "Go version (server): %s\n", v.GoVersion)
	fmt.Fprintf(cli.out, "Git commit (server): %s\n", v.GitCommit)
	fmt.Fprintf(cli.out, "OS/Arch (server): %s/%s\n", v.Os, v.Arch)
	if v.PackageVersion != "" {
		fmt.Fprintf(cli.out, "Server package version: %s\n", v.PackageVersion)
	}

	return nil
}
