package client

import (
	"fmt"
	"io"
	"net/http/httputil"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	Cli "github.com/docker/docker/cli"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/engine-api/types"
)

// CmdAttach attaches to a running container.
//
// Usage: docker attach [OPTIONS] CONTAINER
func (cli *DockerCli) CmdAttach(args ...string) error {
	cmd := Cli.Subcmd("attach", []string{"CONTAINER"}, Cli.DockerCommands["attach"].Description, true)
	noStdin := cmd.Bool([]string{"-no-stdin"}, false, "Do not attach STDIN")
	proxy := cmd.Bool([]string{"-sig-proxy"}, true, "Proxy all received signals to the process")
	detachKeys := cmd.String([]string{"-detach-keys"}, "", "Override the key sequence for detaching a container")

	cmd.Require(flag.Exact, 1)

	cmd.ParseFlags(args, true)

	ctx := context.Background()

	c, err := cli.client.ContainerInspect(ctx, cmd.Arg(0))
	if err != nil {
		return err
	}

	if !c.State.Running {
		return fmt.Errorf("You cannot attach to a stopped container, start it first")
	}

	if c.State.Paused {
		return fmt.Errorf("You cannot attach to a paused container, unpause it first")
	}

	if err := cli.CheckTtyInput(!*noStdin, c.Config.Tty); err != nil {
		return err
	}

	if *detachKeys != "" {
		cli.configFile.DetachKeys = *detachKeys
	}

	container := cmd.Arg(0)

	options := types.ContainerAttachOptions{
		Stream:     true,
		Stdin:      !*noStdin && c.Config.OpenStdin,
		Stdout:     true,
		Stderr:     true,
		DetachKeys: cli.configFile.DetachKeys,
	}

	var in io.ReadCloser
	if options.Stdin {
		in = cli.in
	}

	if *proxy && !c.Config.Tty {
		sigc := cli.forwardAllSignals(ctx, container)
		defer signal.StopCatch(sigc)
	}

	resp, errAttach := cli.client.ContainerAttach(ctx, container, options)
	if errAttach != nil && errAttach != httputil.ErrPersistEOF {
		// ContainerAttach returns an ErrPersistEOF (connection closed)
		// means server met an error and put it in Hijacked connection
		// keep the error and read detailed error message from hijacked connection later
		return errAttach
	}
	defer resp.Close()

	if c.Config.Tty && cli.isTerminalOut {
		height, width := cli.getTtySize()
		// To handle the case where a user repeatedly attaches/detaches without resizing their
		// terminal, the only way to get the shell prompt to display for attaches 2+ is to artificially
		// resize it, then go back to normal. Without this, every attach after the first will
		// require the user to manually resize or hit enter.
		cli.resizeTtyTo(ctx, cmd.Arg(0), height+1, width+1, false)

		// After the above resizing occurs, the call to monitorTtySize below will handle resetting back
		// to the actual size.
		if err := cli.monitorTtySize(ctx, cmd.Arg(0), false); err != nil {
			logrus.Debugf("Error monitoring TTY size: %s", err)
		}
	}
	if err := cli.holdHijackedConnection(ctx, c.Config.Tty, in, cli.out, cli.err, resp); err != nil {
		return err
	}

	if errAttach != nil {
		return errAttach
	}

	_, status, err := cli.getExitCode(ctx, container)
	if err != nil {
		return err
	}
	if status != 0 {
		return Cli.StatusError{StatusCode: status}
	}

	return nil
}
