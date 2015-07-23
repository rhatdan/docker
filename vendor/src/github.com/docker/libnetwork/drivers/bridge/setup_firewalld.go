package bridge

import "github.com/docker/libnetwork/iptables"

func (n *bridgeNetwork) setupFirewalld(config *NetworkConfiguration, i *bridgeInterface) error {
	// Sanity check.
	if config.EnableIPTables == false {
		return IPTableCfgError(config.BridgeName)
	}

	iptables.OnReloaded(func() { setupIPTables(config, i) })
	iptables.OnReloaded(portMapper.ReMapAll)

	return nil
}
