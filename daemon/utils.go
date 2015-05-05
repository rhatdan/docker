package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/nat"
	"github.com/docker/docker/runconfig"
)

func migratePortMappings(config *runconfig.Config, hostConfig *runconfig.HostConfig) error {
	if config.PortSpecs != nil {
		ports, bindings, err := nat.ParsePortSpecs(config.PortSpecs)
		if err != nil {
			return err
		}
		config.PortSpecs = nil
		if len(bindings) > 0 {
			if hostConfig == nil {
				hostConfig = &runconfig.HostConfig{}
			}
			hostConfig.PortBindings = bindings
		}

		if config.ExposedPorts == nil {
			config.ExposedPorts = make(nat.PortSet, len(ports))
		}
		for k, v := range ports {
			config.ExposedPorts[k] = v
		}
	}
	return nil
}

func mergeLxcConfIntoOptions(hostConfig *runconfig.HostConfig) ([]string, error) {
	if hostConfig == nil {
		return nil, nil
	}

	out := []string{}

	// merge in the lxc conf options into the generic config map
	if lxcConf := hostConfig.LxcConf; lxcConf != nil {
		lxSlice := lxcConf.Slice()
		for _, pair := range lxSlice {
			// because lxc conf gets the driver name lxc.XXXX we need to trim it off
			// and let the lxc driver add it back later if needed
			if !strings.Contains(pair.Key, ".") {
				return nil, errors.New("Illegal Key passed into LXC Configurations")
			}
			parts := strings.SplitN(pair.Key, ".", 2)
			out = append(out, fmt.Sprintf("%s=%s", parts[1], pair.Value))
		}
	}

	return out, nil
}

func convertUUID(id string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%.12s", id[0:8], id[8:12], id[12:16], id[16:20], id[20:])
}

func journalPath(id string) string {
	finfo, err := os.Stat("/var/log/journal")
	if err != nil || !finfo.IsDir() {
		return ""
	}

	return fmt.Sprintf("/var/log/journal/%.32s", id)
}
