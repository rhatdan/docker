// +build windows

package daemon

import ()

//Audit/system logging is unsupported in windows environments
func (daemon *Daemon) LogAction(action string, w http.ResponseWriter, id string) error {
	return nil
}
