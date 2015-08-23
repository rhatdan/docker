// +build linux

package server

// #include <stdlib.h>
// #include "/usr/include/pwd.h"
import "C"
import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon"
	"github.com/docker/docker/pkg/audit"
)

//Gets the file descriptor
func getFdFromWriter(w http.ResponseWriter) int {
	//We must use introspection to pull the
	//connection from the ResponseWriter object
	//This is because the connection object is not exported by the writer.
	writerVal := reflect.Indirect(reflect.ValueOf(w))
	//Get the underlying http connection
	httpconn := writerVal.FieldByName("conn")
	httpconnVal := reflect.Indirect(httpconn)
	//Get the underlying tcp connection
	rwcPtr := httpconnVal.FieldByName("rwc").Elem()
	rwc := reflect.Indirect(rwcPtr)
	tcpconn := reflect.Indirect(rwc.FieldByName("conn"))
	//Grab the underyling netfd
	netfd := reflect.Indirect(tcpconn.FieldByName("fd"))
	//Grab sysfd
	sysfd := netfd.FieldByName("sysfd")
	//Finally, we have the fd
	return int(sysfd.Int())
}

//Gets the ucred given an http response writer
func getUcred(fd int) (*syscall.Ucred, error) {
	return syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
}

//Gets the client's loginuid
func getLoginUid(ucred *syscall.Ucred, fd int) (int, error) {
	if _, err := syscall.Getpeername(fd); err != nil {
		logrus.Errorf("Socket appears to have closed: %v", err)
	}
	loginuid, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/loginuid", ucred.Pid))
	if err != nil {
		logrus.Errorf("Error reading loginuid: %v", err)
		return -1, err
	}
	loginuid_int, err := strconv.Atoi(string(loginuid))
	if err != nil {
		logrus.Errorf("Failed to convert loginuid to int: %v", err)
	}
	return loginuid_int, nil
}

//Given a loginUID, retrieves the current username
func getpwuid(loginUID int) (string, error) {
	pw_struct, err := C.getpwuid(C.__uid_t(loginUID))
	if err != nil {
		logrus.Errorf("Failed to get pwuid struct: %v", err)
	}
	name := C.GoString(pw_struct.pw_name)
	return name, nil
}

//Retrieves the container and "action" (start, stop, kill, etc) from the http request
func (s *Server) parseRequest(r *http.Request) (string, *daemon.Container) {
	var (
		containerID string
		action      string
	)
	requrl := r.RequestURI
	parsedurl, err := url.Parse(requrl)
	if err != nil {
		return "?", nil
	}

	switch r.Method {
	//Delete requests do not explicitly state the action, so we check the HTTP method instead
	case "DELETE":
		action = "remove"
		containerID = path.Base(parsedurl.Path)
	default:
		action = path.Base(parsedurl.Path)
		containerID = path.Base(path.Dir(parsedurl.Path))
	}

	c, err := s.daemon.Get(containerID)
	if err != nil {
		return action, nil
	}
	return action, c
}

//Traverses the config struct and grabs non-standard values for logging
func parseConfig(config interface{}) string {
	configReflect := reflect.ValueOf(config)
	var result bytes.Buffer
	for index := 0; index < configReflect.NumField(); index++ {
		val := reflect.Indirect(configReflect.Field(index))
		//Get the zero value of the struct's field
		if val.IsValid() {
			zeroVal := reflect.Zero(val.Type()).Interface()
			//If the configuration value is not a zero value, then we store it
			//We use deep equal here because some types cannot be compared with the standard equality operators
			if val.Kind() == reflect.Bool || !reflect.DeepEqual(zeroVal, val.Interface()) {
				fieldName := configReflect.Type().Field(index).Name
				line := fmt.Sprintf("%s=%+v, ", fieldName, val.Interface())
				result.WriteString(line)
			}
		}
	}
	return result.String()
}

//Constructs a partial log message containing the container's configuration settings
func generateContainerConfigMsg(c *daemon.Container) string {
	if c != nil {
		config_stripped := parseConfig(*c.Config)
		hostConfig_stripped := parseConfig(*c.HostConfig())
		return fmt.Sprintf("Config={%v} HostConfig={%v}", config_stripped, hostConfig_stripped)
	}
	return ""
}

//LogAction logs a docker API function and records the user that initiated the request using the authentication results
func (s *Server) LogAction(w http.ResponseWriter, r *http.Request) error {
	var (
		message  string
		username string
		loginuid int
	)
	action, c := s.parseRequest(r)

	switch action {
	case "start":
		message = fmt.Sprintf("%s%v", message, generateContainerConfigMsg(c))
		fallthrough
	default:
		//Get user credentials
		fd := getFdFromWriter(w)
		ucred, err := getUcred(fd)
		if err != nil {
			break
		}
		message = fmt.Sprintf("PID=%v, %s", ucred.Pid, message)

		//Get user loginuid
		loginuid, err := getLoginUid(ucred, fd)
		if err != nil {
			break
		}
		message = fmt.Sprintf("LoginUID=%v, %s", loginuid, message)

		//Get username
		username, err := getpwuid(loginuid)
		if err != nil {
			break
		}
		message = fmt.Sprintf("Username=%v, %s", username, message)
	}

	//Log the container ID being affected if it exists
	if c != nil {
		message = fmt.Sprintf("ID=%v, %s", c.ID, message)
	}
	message = fmt.Sprintf("{Action=%v, %s}", action, message)
	logSyslog(message)
	logAuditlog(c, action, username, loginuid, true)
	return nil
}

//Logs a message to the syslog
func logSyslog(message string) {
	logger, err := syslog.New(syslog.LOG_ALERT, "Docker")
	defer logger.Close()
	if err != nil {
		fmt.Printf("Error logging to system log: %v", err)
	}
	logger.Info(message)
}

//Logs an API event to the audit log
func logAuditlog(c *daemon.Container, action string, username string, loginuid int, success bool) {
	virt := audit.AUDIT_VIRT_CONTROL
	vm := "?"
	vm_pid := "?"
	exe := "?"
	hostname := "?"
	user := "?"
	auid := "?"

	if c != nil {
		vm = c.Config.Image
		vm_pid = fmt.Sprint(c.State.Pid)
		exe = c.Path
		hostname = c.Config.Hostname
	}

	if username != "" {
		user = username
	}

	if loginuid != -1 {
		auid = fmt.Sprint(loginuid)
	}

	vars := map[string]string{
		"op":       action,
		"reason":   "api",
		"vm":       vm,
		"vm-pid":   vm_pid,
		"user":     user,
		"auid":     auid,
		"exe":      exe,
		"hostname": hostname,
	}

	//Encoding is a function of libaudit that ensures
	//that the audit values contain only approved characters.
	for key, value := range vars {
		if audit.AuditValueNeedsEncoding(value) {
			vars[key] = audit.AuditEncodeNVString(key, value)
		}
	}
	message := audit.AuditFormatVars(vars)
	audit.AuditLogUserEvent(virt, message, success)
}
