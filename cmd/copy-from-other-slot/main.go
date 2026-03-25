package main

// This utility copies a single file from RAUC's respective 'other' slot to the
// host file system. This can for instance be used to determine which software
// version is stored on the 'other' slot.

import (
	"flag"
	"io"
	"log/slog"
	"os"
	"strings"
	"syscall"

	"github.com/holoplot/go-rauc/rauc"
)

func stripQuotes(s string) string {
	s = strings.TrimSuffix(s, "\"")
	s = strings.TrimPrefix(s, "\"")
	return s
}

func main() {
	toFlag := flag.String("to", "", "Destination file point (in the host's root filesystem")
	fromFlag := flag.String("from", "", "File to copy from (in the other slot's filesystem)")
	mountPointFlag := flag.String("mount-point", "/tmp/rauc-other-slot", "Mount point to use temporarily")
	classFlag := flag.String("class", "rootfs", "Slot class to mount")
	flag.Parse()

	if *toFlag == "" || *fromFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	raucInstaller, err := rauc.InstallerNew()
	if err != nil {
		slog.Error("Cannot initialize RAUC installer", "error", err)
		os.Exit(1)
	}

	statuses, err := raucInstaller.GetSlotStatus()
	if err != nil {
		slog.Error("Cannot get slot statuses", "error", err)
		os.Exit(1)
	}

	for _, status := range statuses {
		if s, ok := status.Status["class"]; !ok || stripQuotes(s.String()) != *classFlag {
			continue
		}

		s, ok := status.Status["state"]
		if !ok {
			continue
		}
		state := stripQuotes(s.String())

		if state == "booted" {
			continue
		}

		device := stripQuotes(status.Status["device"].String())
		slog.Info("Device path for mount", "device", device)

		if err := os.MkdirAll(*mountPointFlag, 0755); err != nil && err != os.ErrExist {
			slog.Error("MkdirTemp() failed", "error", err)

			return
		}

		if err = syscall.Mount(device, *mountPointFlag, "squashfs", 0, ""); err != nil {
			slog.Error("Unable to mount", "error", err, "device", device, "mountPoint", *mountPointFlag)

			return
		}

		slog.Info("Successfully mounted", "device", device, "mountPoint", *mountPointFlag)

		defer syscall.Unmount(*mountPointFlag, 0)

		from, err := os.Open(*mountPointFlag + *fromFlag)
		if err != nil {
			slog.Error("Cannot open file", "error", err, "from", *fromFlag)
			return
		}

		defer from.Close()

		to, err := os.OpenFile(*toFlag, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			slog.Error("Cannot open file", "error", err, "to", *toFlag)
			return
		}

		defer to.Close()

		_, err = io.Copy(to, from)
		if err != nil {
			slog.Error("Cannot copy file content", "error", err, "to", *toFlag, "from", *fromFlag)
			return
		}

		slog.Info("Successfully copied", "to", *toFlag, "from", *fromFlag, "class", *classFlag)
	}
}
