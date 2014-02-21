package systemd

import (
	"fmt"
	"github.com/guelfey/go.dbus"
	"github.com/guelfey/go.dbus/introspect"
	"sync"
)

const (
	SystemdName = "org.freedesktop.systemd1"
)

type Property struct {
	Name  string
	Value interface{}
}

type DeviceAllow struct {
	Node        string
	Permissions string
}

type dbusProperty struct {
	Name  string
	Value dbus.Variant
}

func (p Property) toDbus() dbusProperty {
	return dbusProperty{p.Name, dbus.MakeVariant(p.Value)}
}

type auxItem struct {
	Name       string
	Properties []dbusProperty
}

type Manager struct {
	*dbus.Object
	conn     *dbus.Conn
	jobs     map[dbus.ObjectPath]chan string
	jobsLock sync.Mutex

	HasStartTransientUnit bool
}

var (
	managerLock sync.Mutex
	theManager  *Manager
)

func (m *Manager) jobRemoved(signal *dbus.Signal) {
	var (
		id     uint32
		job    dbus.ObjectPath
		unit   string
		result string
	)

	dbus.Store(signal.Body, &id, &job, &unit, &result)

	m.jobsLock.Lock()
	ch, ok := m.jobs[job]
	if ok {
		delete(m.jobs, job)
		ch <- result
	}
	m.jobsLock.Unlock()
}

func (m *Manager) handleSignals(ch chan *dbus.Signal) {
	for {
		signal := <-ch
		switch signal.Name {
		case "org.freedesktop.systemd1.Manager.JobRemoved":
			m.jobRemoved(signal)
		}
	}
}

func GetManager() (*Manager, error) {
	managerLock.Lock()
	defer managerLock.Unlock()

	if theManager != nil {
		return theManager, nil
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object(SystemdName, "/org/freedesktop/systemd1")

	introspectData, err := introspect.Call(obj)
	if err != nil {
		return nil, err
	}

	hasStartTransientUnit := false
	for _, i := range introspectData.Interfaces {
		if i.Name == "org.freedesktop.systemd1.Manager" {
			for _, m := range i.Methods {
				if m.Name == "StartTransientUnit" {
					hasStartTransientUnit = true
				}
			}
		}
	}

	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal', interface='org.freedesktop.systemd1.Manager', member='JobRemoved'")

	signalCh := make(chan *dbus.Signal, 100)

	conn.Signal(signalCh)

	theManager = &Manager{
		Object: obj,
		conn:   conn,
		jobs:   make(map[dbus.ObjectPath]chan string),
		HasStartTransientUnit: hasStartTransientUnit,
	}

	go theManager.handleSignals(signalCh)

	return theManager, nil
}

func (m *Manager) StartTransientUnit(name, mode string, properties []Property) error {
	dbusProperties := make([]dbusProperty, len(properties))
	for i, p := range properties {
		dbusProperties[i] = p.toDbus()
	}

	ch := make(chan string, 1)

	m.jobsLock.Lock()
	var path dbus.ObjectPath
	err := m.Call(method("Manager", "StartTransientUnit"), 0, name, mode, dbusProperties, []auxItem{}).Store(&path)
	if err != nil {
		m.jobsLock.Unlock()
		return err
	}
	m.jobs[path] = ch
	m.jobsLock.Unlock()

	// Wait for job to be removed
	res := <-ch

	if res != "done" {
		return fmt.Errorf("StartTransientUnit job failed with status %s", res)
	}
	return nil
}

func (m *Manager) GetUnit(name string) (*Unit, error) {
	var path dbus.ObjectPath

	err := m.Call(method("Manager", "GetUnit"), 0, name).Store(&path)
	if err != nil {
		return nil, err
	}

	return newUnit(path)
}

type Unit struct {
	*dbus.Object
}

func method(iface, method string) string {
	return SystemdName + "." + iface + "." + method
}

func newUnit(path dbus.ObjectPath) (*Unit, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	return &Unit{conn.Object(SystemdName, path)}, nil
}

func (u *Unit) GetProperty(iface, property string) (interface{}, error) {
	var res dbus.Variant
	if err := u.Call("org.freedesktop.DBus.Properties.Get", 0, iface, property).Store(&res); err != nil {
		return nil, err
	}
	return res.Value(), nil
}

func (u *Unit) Kill(who string, signal int) error {
	return u.Call(method("Unit", "Kill"), 0, who, signal).Store()
}
