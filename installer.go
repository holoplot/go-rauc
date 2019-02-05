package rauc

import (
	"errors"
	"fmt"
	"github.com/godbus/dbus"
)

type Installer struct {
	conn   *dbus.Conn
	object  dbus.BusObject
}

const (
	dbusInterface = "de.pengutronix.rauc"
)

type SlotStatus struct {
	SlotName string
	Status   map[string]dbus.Variant
}

func InstallerNew() (*Installer, error) {
	p := new(Installer)
	var err error
	p.conn, err = dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	p.object = p.conn.Object(dbusInterface, dbus.ObjectPath("/"))
	p.object.AddMatchSignal(fmt.Sprintf("%s.%s", dbusInterface, "Installer"), "Completed",
							dbus.WithMatchObjectPath(p.object.Path()))

	return p, nil
}

func (p *Installer) interfaceForMember(method string) string {
	return fmt.Sprintf("%s.%s.%s", dbusInterface, "Installer", method)
}

func (p *Installer) Install(filename string) error {
	doneChannel := make(chan *dbus.Signal, 10)
	p.conn.Signal(doneChannel)

	err := p.object.Call(p.interfaceForMember("Install"), 0, filename).Err
	if err != nil {
		return err
	}

	for {
		signal, ok := <- doneChannel
		if !ok {
			return errors.New("Cannot read from channel")
		}

		if signal.Name == p.interfaceForMember("Completed") {
			var code int32
			err = dbus.Store(signal.Body, &code)
			if err != nil {
				return err
			}

			if code != 0 {
				errorString, err := p.GetLastError()
				if err != nil {
					return err
				}

				return errors.New(errorString)
			}

			return nil
		}
	}
}

func (p *Installer) Info(filename string) (compatible string, version string, err error) {
	err = p.object.Call(p.interfaceForMember("Info"), 0, filename).Store(&compatible, &version)
	if err != nil {
		return "", "", err
	}

	return compatible, version, nil
}

func (p *Installer) Mark(state string, slotIdentifier string) (slotName string, message string, err error) {
	err = p.object.Call(p.interfaceForMember("Mark"), 0, state, slotIdentifier).Store(&slotName, &message)
	if err != nil {
		return "", "", err
	}

	return slotName, message, nil
}

func (p *Installer) GetSlotStatus() (status []SlotStatus, err error) {
	err = p.object.Call(p.interfaceForMember("GetSlotStatus"), 0).Store(&status)
	if err != nil {
		fmt.Printf("ERROR:", err.Error())
		return nil, err
	}

	return status, nil
}

// Properties

func (p *Installer) GetOperation() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Operation"))
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (p *Installer) GetLastError() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("LastError"))
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (p *Installer) GetProgress() (percentage int32, message string, nestingDepth int32, err error) {
	variant, err := p.object.GetProperty(p.interfaceForMember("Progress"))
	if err != nil {
		return -1, "", -1, err
	}

	type progressResponse struct {
		percentage   int32
		message      string
		nestingDepth int32
	}

	src := make([]interface{}, 1)
	src[0] = variant.Value()

	var response progressResponse
	dbus.Store(src, &response)

	return response.percentage, response.message, response.nestingDepth, nil
}

func (p *Installer) GetCompatible() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Compatible"))
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (p *Installer) GetVariant() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Variant"))
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (p *Installer) GetBootSlot() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("BootSlot"))
	if err != nil {
		return "", err
	}

	return v.String(), nil
}
