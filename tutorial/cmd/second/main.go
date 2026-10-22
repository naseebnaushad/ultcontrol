// second program: ask several questions using a helper function.
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

	var dec frame.Decoder
	seq := byte(0)

	// ask sends one command and waits for the reply whose first byte is replyCmd.
	ask := func(payload []byte, replyCmd byte) []byte {
		f.Write(frame.Encode(frame.TypeMDR, seq, payload))
		seq ^= 1 // the sequence number alternates 0, 1, 0, 1 ...
		buf := make([]byte, 256)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			n, err := f.Read(buf)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for _, fr := range dec.Feed(buf[:n]) {
				if fr.DataType != frame.TypeMDR {
					continue
				}
				f.Write(frame.Encode(frame.TypeAck, fr.Seq^1, nil)) // always acknowledge
				if len(fr.Payload) > 0 && fr.Payload[0] == replyCmd {
					return fr.Payload
				}
			}
		}
		return nil
	}

	if r := ask([]byte{0x04, 0x01}, 0x05); r != nil {
		fmt.Printf("name:    %s\n", r[3:])
	}
	if r := ask([]byte{0x22, 0x00}, 0x23); r != nil {
		fmt.Printf("battery: %d%%\n", r[2])
	}
	if r := ask([]byte{0x66, 0x17}, 0x67); r != nil {
		mode := "noise cancelling"
		if r[4] == 1 {
			mode = "ambient sound"
		}
		fmt.Printf("mode:    %s, ambient level %d\n", mode, r[6])
	}
}
