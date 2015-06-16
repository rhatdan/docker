// +build windows

package daemon

import ()

//Audit/system logging is unsupported in windows environments
func (daemon *Daemon) LogAction(w http.ResponseWriter, action string, id string) error {
	return nil
}
