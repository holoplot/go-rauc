package rauc

import (
	"context"
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

	// propertiesInterface is the standard D-Bus properties interface, used to
	// receive PropertiesChanged notifications for Progress.
	propertiesInterface = "org.freedesktop.DBus.Properties"
)

// Mark states accepted by Mark().
const (
	// MarkGood keeps a slot bootable and, for bootloaders that count boot
	// attempts, resets the remaining attempts for that slot.
	MarkGood = "good"
	// MarkBad makes a slot unbootable.
	MarkBad = "bad"
	// MarkActive explicitly activates a slot for the next boot.
	MarkActive = "active"
)

// Slot identifiers accepted by Mark(). A specific slot may also be named
// directly, in <class>.<index> form, e.g. "rootfs.0".
const (
	// SlotBooted refers to the currently booted slot.
	SlotBooted = "booted"
	// SlotOther refers to the slot that is not currently booted. Only
	// meaningful on a system with exactly two slots of that class.
	SlotOther = "other"
)

// SlotStatus is returned by .GetSlotStatus() and contains information
// on the status of an available boot slots.
type SlotStatus struct {
	SlotName string
	Status   map[string]dbus.Variant
}

// Progress describes how far a running installation has got.
type Progress struct {
	Percentage   int32
	Message      string
	NestingDepth int32
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
	if err := p.conn.AddMatchSignal(
		dbus.WithMatchInterface(fmt.Sprintf("%s.%s", dbusInterface, "Installer")),
		dbus.WithMatchMember("Completed"),
		dbus.WithMatchObjectPath(p.object.Path())); err != nil {
		return nil, fmt.Errorf("RAUC: cannot subscribe to Completed: %v", err)
	}

	return p, nil
}

// Close releases this Installer's reference to the system bus.
func (p *Installer) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

func (p *Installer) interfaceForMember(method string) string {
	return fmt.Sprintf("%s.%s.%s", dbusInterface, "Installer", method)
}

// stringProperty reads a string-typed property.
//
// Variant.String() renders the D-Bus representation of a value, which for a
// string includes surrounding quotes, so it cannot be used to read the value
// itself.
func (p *Installer) stringProperty(name string) (string, error) {
	v, err := p.object.GetProperty(p.interfaceForMember(name))
	if err != nil {
		return "", fmt.Errorf("RAUC: GetProperty(%s): %v", name, err)
	}

	s, ok := v.Value().(string)
	if !ok {
		return "", fmt.Errorf("RAUC: GetProperty(%s): expected string, got %T", name, v.Value())
	}

	return s, nil
}

// BundleAccessOptions describes how to reach a bundle that is served over the
// network. It is shared by the methods that take a bundle source. All fields
// are optional; unset ones are not sent to the daemon, which then applies its
// own defaults.
type BundleAccessOptions struct {
	// TLSCert is the file path or PKCS#11 URL of the client certificate to
	// authenticate with against the server.
	TLSCert string

	// TLSKey is the file path or PKCS#11 URL of the private key belonging to
	// TLSCert.
	TLSKey string

	// TLSCA is the file path of the CA certificate used to authenticate the
	// server, in place of the system's trust store.
	TLSCA string

	// TLSNoVerify disables verification of the server certificate.
	TLSNoVerify bool

	// HTTPHeaders are additional HTTP headers to send with every request, each
	// in "Name: value" form, e.g. to pass a bearer token.
	HTTPHeaders []string
}

func (o BundleAccessOptions) apply(args map[string]any) {
	if o.TLSCert != "" {
		args["tls-cert"] = o.TLSCert
	}
	if o.TLSKey != "" {
		args["tls-key"] = o.TLSKey
	}
	if o.TLSCA != "" {
		args["tls-ca"] = o.TLSCA
	}
	if o.TLSNoVerify {
		args["tls-no-verify"] = true
	}
	if len(o.HTTPHeaders) > 0 {
		args["http-headers"] = o.HTTPHeaders
	}
}

// InstallBundleOptions contains options for the InstallBundle method
type InstallBundleOptions struct {
	// IgnoreIncompatible installs a bundle whose compatible string does not
	// match the one of the running system.
	IgnoreIncompatible bool

	// IgnoreVersionLimit disables the minimum bundle version check configured
	// in system.conf.
	IgnoreVersionLimit bool

	// TransactionID is a caller-chosen UUID identifying this installation. If
	// empty, the daemon generates one.
	TransactionID string

	// RequireManifestHash aborts the installation unless the bundle's manifest
	// hash matches this value.
	RequireManifestHash string

	BundleAccessOptions
}

func (o InstallBundleOptions) args() map[string]any {
	args := map[string]any{
		"ignore-compatible": o.IgnoreIncompatible,
	}
	if o.IgnoreVersionLimit {
		args["ignore-version-limit"] = true
	}
	if o.TransactionID != "" {
		args["transaction-id"] = o.TransactionID
	}
	if o.RequireManifestHash != "" {
		args["require-manifest-hash"] = o.RequireManifestHash
	}
	o.BundleAccessOptions.apply(args)

	return args
}

// InspectBundleOptions contains options for the InspectBundle method
type InspectBundleOptions struct {
	BundleAccessOptions
}

func (o InspectBundleOptions) args() map[string]any {
	args := map[string]any{}
	o.BundleAccessOptions.apply(args)

	return args
}

// InstallBundle triggers the installation of a bundle. This method waits for the "Completed"
// signal to be sent by the RAUC daemon.
func (p *Installer) InstallBundle(filename string, options InstallBundleOptions) error {
	return p.InstallBundleContext(context.Background(), filename, options)
}

// InstallBundleContext behaves like InstallBundle but abandons the wait when
// ctx is done.
//
// Cancelling ctx stops this call waiting; it does not abort the installation,
// which continues in the daemon.
func (p *Installer) InstallBundleContext(ctx context.Context, filename string, options InstallBundleOptions) error {
	// Subscribed before the call is issued so that an installation which fails
	// immediately cannot emit Completed before anything is listening.
	doneChannel := make(chan *dbus.Signal, 10)
	p.conn.Signal(doneChannel)
	defer p.conn.RemoveSignal(doneChannel)

	err := p.object.CallWithContext(ctx, p.interfaceForMember("InstallBundle"), 0, filename, options.args()).Err
	if err != nil {
		return fmt.Errorf("RAUC: Install(): %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case signal, ok := <-doneChannel:
			if !ok {
				return errors.New("RAUC: Cannot read from channel")
			}

			if signal == nil || signal.Name != p.interfaceForMember("Completed") {
				continue
			}

			var code int32
			err = dbus.Store(signal.Body, &code)
			if err != nil {
				return err
			}

			if code != 0 {
				errorString, err := p.GetLastError()
				if err != nil {
					return fmt.Errorf("RAUC: install failed with code %d", code)
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

// InspectBundle returns the manifest and metadata of a bundle without
// installing it. It accepts the same source forms as InstallBundle, including
// network locations, and abandons the call when ctx is done.
func (p *Installer) InspectBundle(ctx context.Context, filename string, options InspectBundleOptions) (map[string]dbus.Variant, error) {
	var info map[string]dbus.Variant
	err := p.object.CallWithContext(ctx, p.interfaceForMember("InspectBundle"), 0, filename, options.args()).Store(&info)
	if err != nil {
		return nil, fmt.Errorf("RAUC: InspectBundle(): %v", err)
	}

	return info, nil
}

// Mark keeps a slot bootable (state == “good”), makes it unbootable (state == “bad”)
// or explicitly activates it for the next boot (state == “active”).
//
// Use the MarkGood, MarkBad and MarkActive constants for state, and SlotBooted
// or SlotOther for slotIdentifier to avoid having to determine the slot name.
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

// GetPrimary returns the slot the bootloader will boot next.
//
// After an installation this is the newly written slot, which is not yet the
// booted one; compare with GetBootSlot to tell whether a reboot has happened.
func (p *Installer) GetPrimary() (string, error) {
	var primary string
	err := p.object.Call(p.interfaceForMember("GetPrimary"), 0).Store(&primary)
	if err != nil {
		return "", fmt.Errorf("RAUC: GetPrimary(): %v", err)
	}

	return primary, nil
}

// Properties

// GetOperation returns the current (global) operation RAUC performs.
func (p *Installer) GetOperation() (string, error) {
	return p.stringProperty("Operation")
}

// GetLastError returns the last message of the last error that occurred.
func (p *Installer) GetLastError() (string, error) {
	return p.stringProperty("LastError")
}

// GetProgress returns installation progress information in the form
// (percentage, message, nesting depth)
func (p *Installer) GetProgress() (percentage int32, message string, nestingDepth int32, err error) {
	variant, err := p.object.GetProperty(p.interfaceForMember("Progress"))
	if err != nil {
		return -1, "", -1, fmt.Errorf("RAUC: GetProperty(Progress): %v", err)
	}

	src := make([]any, 1)
	src[0] = variant.Value()

	var response Progress
	err = dbus.Store(src, &response)
	if err != nil {
		return -1, "", -1, fmt.Errorf("RAUC: Cannot store result: %v", err)
	}

	return response.Percentage, response.Message, response.NestingDepth, nil
}

// WatchProgress delivers progress updates as the daemon reports them, so
// callers do not have to poll GetProgress.
//
// The returned channel is closed when ctx is done. Updates are dropped rather
// than queued if the receiver is not keeping up, so a slow consumer cannot
// stall the bus connection; the final value is always delivered by the
// Completed signal that InstallBundle returns on.
func (p *Installer) WatchProgress(ctx context.Context) (<-chan Progress, error) {
	matchOptions := []dbus.MatchOption{
		dbus.WithMatchInterface(propertiesInterface),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(p.object.Path()),
	}

	if err := p.conn.AddMatchSignal(matchOptions...); err != nil {
		return nil, fmt.Errorf("RAUC: cannot subscribe to PropertiesChanged: %v", err)
	}

	signals := make(chan *dbus.Signal, 16)
	p.conn.Signal(signals)

	updates := make(chan Progress, 16)

	go func() {
		defer func() {
			_ = p.conn.RemoveMatchSignal(matchOptions...)
			p.conn.RemoveSignal(signals)
			close(updates)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				progress, found := progressFromPropertiesChanged(signal)
				if !found {
					continue
				}
				select {
				case updates <- progress:
				default:
				}
			}
		}
	}()

	return updates, nil
}

// progressFromPropertiesChanged extracts a Progress value from a
// PropertiesChanged signal, reporting whether one was present.
func progressFromPropertiesChanged(signal *dbus.Signal) (Progress, bool) {
	if signal == nil || signal.Name != propertiesInterface+".PropertiesChanged" {
		return Progress{}, false
	}

	var (
		interfaceName string
		changed       map[string]dbus.Variant
		invalidated   []string
	)
	if err := dbus.Store(signal.Body, &interfaceName, &changed, &invalidated); err != nil {
		return Progress{}, false
	}

	if interfaceName != fmt.Sprintf("%s.%s", dbusInterface, "Installer") {
		return Progress{}, false
	}

	variant, ok := changed["Progress"]
	if !ok {
		return Progress{}, false
	}

	var progress Progress
	if err := dbus.Store([]any{variant.Value()}, &progress); err != nil {
		return Progress{}, false
	}

	return progress, true
}

// GetCompatible returns the system’s compatible string.
// This can be used to check for usable bundels.
func (p *Installer) GetCompatible() (string, error) {
	return p.stringProperty("Compatible")
}

// GetVariant returns the system’s variant.
// This can be used to select parts of an bundle.
func (p *Installer) GetVariant() (string, error) {
	return p.stringProperty("Variant")
}

// GetBootSlot returns the currently used boot slot.
func (p *Installer) GetBootSlot() (string, error) {
	return p.stringProperty("BootSlot")
}
