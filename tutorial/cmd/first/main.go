// first contact: connect, ask for the name, print the reply.
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"learn/frame"
)

const sonyUUID = "956c7b26-d49a-4ba8-b03f-b17d393cb6e2"

// profile is the object BlueZ calls when the connection is ready.
type profile struct{ conns chan *os.File }

func (p *profile) NewConnection(_ dbus.ObjectPath, fd dbus.UnixFD, _ map[string]dbus.Variant) *dbus.Error {
	syscall.SetNonblock(int(fd), true)
	p.conns <- os.NewFile(uintptr(fd), "rfcomm")
	return nil
}
func (p *profile) RequestDisconnection(dbus.ObjectPath) *dbus.Error { return nil }
func (p *profile) Release() *dbus.Error                             { return nil }

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: first AA:BB:CC:DD:EE:FF")
		os.Exit(1)
	}
	mac := os.Args[1]

	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		panic(err)
	}
	defer bus.Close()

	p := &profile{conns: make(chan *os.File, 1)}
	path := dbus.ObjectPath("/learn/profile")
	bus.Export(p, path, "org.bluez.Profile1")

	opts := map[string]dbus.Variant{
		"Role":                  dbus.MakeVariant("client"),
		"RequireAuthentication": dbus.MakeVariant(false),
		"RequireAuthorization":  dbus.MakeVariant(false),
		"AutoConnect":           dbus.MakeVariant(false),
	}
	mgr := bus.Object("org.bluez", "/org/bluez")
	if err := mgr.Call("org.bluez.ProfileManager1.RegisterProfile", 0, path, sonyUUID, opts).Err; err != nil {
		panic(err)
	}
	defer mgr.Call("org.bluez.ProfileManager1.UnregisterProfile", 0, path)

	dev := dbus.ObjectPath("/org/bluez/hci0/dev_" + strings.ReplaceAll(mac, ":", "_"))
	bus.Object("org.bluez", dev).Go("org.bluez.Device1.ConnectProfile", 0, nil, sonyUUID)

	var f *os.File
	select {
	case f = <-p.conns:
	case <-time.After(10 * time.Second):
		fmt.Println("no connection (is another app using the headphones?)")
		os.Exit(1)
	}
	defer f.Close()

	f.Write(frame.Encode(frame.TypeMDR, 0, []byte{0x04, 0x01})) // "what is your name?"

	var dec frame.Decoder
	buf := make([]byte, 256)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := f.Read(buf)
		if err != nil {
			time.Sleep(50 * time.Millisecond) // nothing yet, try again
			continue
		}
		for _, fr := range dec.Feed(buf[:n]) {
			if fr.DataType != frame.TypeMDR {
				continue // an acknowledgement; ignore it
			}
			fmt.Printf("name: %s\n", fr.Payload[3:])
			f.Write(frame.Encode(frame.TypeAck, fr.Seq^1, nil))
			return
		}
	}
}
