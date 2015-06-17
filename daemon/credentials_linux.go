// +build linux
package daemon

// #include <stdlib.h>
// #include "/usr/include/pwd.h"
import "C"
import (
	"bytes"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/audit"
	"io/ioutil"
	"log/syslog"
	"net/http"
	"reflect"
	"strconv"
	"syscall"
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

func (daemon *Daemon) LogAction(w http.ResponseWriter, action string, id string) error {
	var message string
	switch action {
	//If the event we are logging should
	//have the configuration attached
	case "create", "start":
		c, err := daemon.Get(id)
		if err == nil {
			config_stripped := parseConfig(*c.Config)
			hostConfig_stripped := parseConfig(*c.hostConfig)
			message += fmt.Sprintf("Config=%v HostConfig=%v", config_stripped, hostConfig_stripped)
		}
		fallthrough
	//Non-creation events don't need
	//the entire configuration logged
	default:
		//Get user credentials
		fd := getFdFromWriter(w)
		ucred, err := getUcred(fd)
		if err != nil {
			break
		}
		message = fmt.Sprintf("PID=%v, ", ucred.Pid) + message

		//Get user loginuid
		loginuid, err := getLoginUid(ucred, fd)
		if err != nil {
			break
		}
		message = fmt.Sprintf("LoginUID=%v, ", loginuid) + message

		//Get username
		username, err := getpwuid(loginuid)
		if err != nil {
			break
		}
		message = fmt.Sprintf("Username=%v, ", username) + message
	}
	//Wrap everything in brackets and append the acction and ID
	message = fmt.Sprintf("{Action=%v, ID=%s, %s}", action, id, message)
	logSyslog(message)
	audit.AuditLogUserEvent(audit.AUDIT_VIRT_CONTROL, message, true)
	return nil
}

//Logs a message to the syslog
func logSyslog(message string) {
	logger, err := syslog.New(syslog.LOG_ALERT, "Docker")
	defer logger.Close()
	if err != nil {
		fmt.Printf("Error logging to syslog: %v", err)
	}
	logger.Info(message)
}
