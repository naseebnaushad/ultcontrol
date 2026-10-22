package sony

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	sonyUUID    = "956c7b26-d49a-4ba8-b03f-b17d393cb6e2"
	profilePath = dbus.ObjectPath("/ultcontrol/profile")
)

type profile struct{ conns chan *os.File }

// NewConnection is called by BlueZ with the connected RFCOMM socket.
func (p *profile) NewConnection(_ dbus.ObjectPath, fd dbus.UnixFD, _ map[string]dbus.Variant) *dbus.Error {
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		return dbus.MakeFailedError(err)
	}
	p.conns <- os.NewFile(uintptr(fd), "rfcomm")
	return nil
}

func (p *profile) RequestDisconnection(dbus.ObjectPath) *dbus.Error { return nil }
func (p *profile) Release() *dbus.Error                             { return nil }

// dial asks BlueZ to open the Sony control profile on the device and returns the socket.
func dial(ctx context.Context, mac string) (*os.File, error) {
	// The headphones refuse a new control connection for a few seconds after the previous one
	// closed ("br-connection-busy"), so retry until the context expires.
	for {
		f, err := dialOnce(ctx, mac)
		if err == nil || !strings.Contains(err.Error(), "busy") || ctx.Err() != nil {
			return f, err
		}
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-ctx.Done():
			return nil, err
		}
	}
}

var macRe = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

func dialOnce(ctx context.Context, mac string) (*os.File, error) {
	// mac becomes part of a D-Bus object path, so refuse anything that is not an address.
	if !macRe.MatchString(mac) {
		return nil, fmt.Errorf("invalid Bluetooth address %q", mac)
	}
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	defer bus.Close()

	p := &profile{conns: make(chan *os.File, 1)}
	if err := bus.Export(p, profilePath, "org.bluez.Profile1"); err != nil {
		return nil, err
	}
	opts := map[string]dbus.Variant{
		"Role":                  dbus.MakeVariant("client"),
		"RequireAuthentication": dbus.MakeVariant(false),
		"RequireAuthorization":  dbus.MakeVariant(false),
		"AutoConnect":           dbus.MakeVariant(false),
	}
	mgr := bus.Object("org.bluez", "/org/bluez")
	if err := mgr.Call("org.bluez.ProfileManager1.RegisterProfile", 0, profilePath, sonyUUID, opts).Err; err != nil {
		return nil, err
	}
	defer mgr.Call("org.bluez.ProfileManager1.UnregisterProfile", 0, profilePath)

	devPath := dbus.ObjectPath("/org/bluez/hci0/dev_" + strings.ReplaceAll(mac, ":", "_"))
	call := bus.Object("org.bluez", devPath).GoWithContext(ctx, "org.bluez.Device1.ConnectProfile", 0, nil, sonyUUID)
	select {
	case f := <-p.conns:
		return f, nil
	case <-call.Done:
		if call.Err != nil {
			return nil, call.Err
		}
		select {
		case f := <-p.conns:
			return f, nil
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
	return nil, fmt.Errorf("could not open control channel to %s (is it connected and not in use by another app?)", mac)
}

// FindDevice returns the address of the first paired Sony ULT headphone known to BlueZ.
func FindDevice() (string, error) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return "", err
	}
	defer bus.Close()
	var objs map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := bus.Object("org.bluez", "/").Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objs); err != nil {
		return "", err
	}
	for _, ifaces := range objs {
		dev, ok := ifaces["org.bluez.Device1"]
		if !ok {
			continue
		}
		name, _ := dev["Name"].Value().(string)
		addr, _ := dev["Address"].Value().(string)
		if strings.HasPrefix(name, "ULT") && addr != "" {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no paired ULT headphones found; pass --mac")
}
