package rauc

import (
	"errors"
	"fmt"

	dbus "github.com/godbus/dbus/v5"
)

// Installer is the central object interface that handles
// all communication with the RAUC daemon
type Installer struct {
	conn   *dbus.Conn
	object dbus.BusObject
}

const (
	dbusInterface = "de.pengutronix.rauc"
)

// SlotStatus is returned by .GetSlotStatus() and contains information
// on the status of an available boot slots.
type SlotStatus struct {
	SlotName string
	Status   map[string]dbus.Variant
}

// InstallerNew returns a newly allocated Installer object
func InstallerNew() (*Installer, error) {
	p := new(Installer)
	var err error
	p.conn, err = dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	p.object = p.conn.Object(dbusInterface, dbus.ObjectPath("/"))
	p.conn.AddMatchSignal(
		dbus.WithMatchInterface(fmt.Sprintf("%s.%s", dbusInterface, "Installer")),
		dbus.WithMatchMember("Completed"),
		dbus.WithMatchObjectPath(p.object.Path()))

	return p, nil
}

func (p *Installer) interfaceForMember(method string) string {
	return fmt.Sprintf("%s.%s.%s", dbusInterface, "Installer", method)
}

// InstallBundleOptions contains options for the InstallBundle method
type InstallBundleOptions struct {
	IgnoreIncompatible bool
}

// InstallBundle triggers the installation of a bundle. This method waits for the "Completed"
// signal to be sent by the RAUC daemon.
func (p *Installer) InstallBundle(filename string, options InstallBundleOptions) error {
	doneChannel := make(chan *dbus.Signal, 10)
	p.conn.Signal(doneChannel)

	args := map[string]interface{}{
		"ignore-compatible": options.IgnoreIncompatible,
	}

	err := p.object.Call(p.interfaceForMember("InstallBundle"), 0, filename, args).Err
	if err != nil {
		return fmt.Errorf("RAUC: Install(): %v", err)
	}

	for {
		signal, ok := <-doneChannel
		if !ok {
			return errors.New("RAUC: Cannot read from channel")
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

// Info provides information on a given bundle.
func (p *Installer) Info(filename string) (compatible string, version string, err error) {
	err = p.object.Call(p.interfaceForMember("Info"), 0, filename).Store(&compatible, &version)
	if err != nil {
		return "", "", fmt.Errorf("RAUC: Info(): %v", err)
	}

	return compatible, version, nil
}

// Mark keeps a slot bootable (state == “good”), makes it unbootable (state == “bad”)
// or explicitly activates it for the next boot (state == “active”).
func (p *Installer) Mark(state string, slotIdentifier string) (slotName string, message string, err error) {
	err = p.object.Call(p.interfaceForMember("Mark"), 0, state, slotIdentifier).Store(&slotName, &message)
	if err != nil {
		return "", "", fmt.Errorf("RAUC: Mark(): %v", err)
	}

	return slotName, message, nil
}

// GetSlotStatus is an access method to get all slots’ status.
func (p *Installer) GetSlotStatus() (status []SlotStatus, err error) {
	err = p.object.Call(p.interfaceForMember("GetSlotStatus"), 0).Store(&status)
	if err != nil {
		return nil, fmt.Errorf("RAUC: GetSlotStatus(): %v", err)
	}

	return status, nil
}

// Properties

// GetOperation returns the current (global) operation RAUC performs.
func (p *Installer) GetOperation() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Operation"))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetOperation(): %v", err)
	}

	return v.String(), nil
}

// GetLastError returns the last message of the last error that occurred.
func (p *Installer) GetLastError() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("LastError"))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetLastError(): %v", err)
	}

	return v.String(), nil
}

// GetProgress returns installation progress information in the form
// (percentage, message, nesting depth)
func (p *Installer) GetProgress() (percentage int32, message string, nestingDepth int32, err error) {
	variant, err := p.object.GetProperty(p.interfaceForMember("Progress"))
	if err != nil {
		return -1, "", -1, fmt.Errorf("RAUC: GetProperty(Progress): %v", err)
	}

	type progressResponse struct {
		Percentage   int32
		Message      string
		NestingDepth int32
	}

	src := make([]interface{}, 1)
	src[0] = variant.Value()

	var response progressResponse
	err = dbus.Store(src, &response)
	if err != nil {
		return -1, "", -1, fmt.Errorf("RAUC: Cannot store result: %v", err)
	}

	return response.Percentage, response.Message, response.NestingDepth, nil
}

// GetCompatible returns the system’s compatible string.
// This can be used to check for usable bundels.
func (p *Installer) GetCompatible() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Compatible"))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetProperty(Compatible): %v", err)
	}

	return v.String(), nil
}

// GetVariant returns the system’s variant.
// This can be used to select parts of an bundle.
func (p *Installer) GetVariant() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("Variant"))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetProperty(Variant): %v", err)
	}

	return v.String(), nil
}

// GetBootSlot returns the currently used boot slot.
func (p *Installer) GetBootSlot() (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember("BootSlot"))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetProperty(BootSlot): %v", err)
	}

	return v.String(), nil
}
