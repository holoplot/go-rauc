package main

// This utility copies a single file from RAUC's respective 'other' slot to the
// host file system. This can for instance be used to determine which software
// version is stored on the 'other' slot.

import (
	"flag"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/holoplot/go-rauc/rauc"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func stripQuotes(s string) string {
	s = strings.TrimSuffix(s, "\"")
	s = strings.TrimPrefix(s, "\"")
	return s
}

func main() {
	consoleWriter := zerolog.ConsoleWriter{
		Out: colorable.NewColorableStdout(),
	}

	if isatty.IsTerminal(os.Stdout.Fd()) {
		consoleWriter.TimeFormat = time.RFC3339
	}

	log.Logger = log.Output(consoleWriter)

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
		log.Fatal().
			Err(err).
			Msg("Cannot initialize")
	}

	statuses, err := raucInstaller.GetSlotStatus()
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Cannot get slot statuses")
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
		log.Info().
			Str("device", device).
			Msg("Device path for mount")

		if err := os.MkdirAll(*mountPointFlag, 0755); err != nil && err != os.ErrExist {
			log.Error().
				Err(err).
				Msg("MkdirTemp() failed")
			return
		}

		if err = syscall.Mount(device, *mountPointFlag, "squashfs", 0, ""); err != nil {
			log.Error().
				Err(err).
				Str("device", device).
				Str("mountPoint", *mountPointFlag).
				Msg("Unable to mount")
			return
		}

		log.Info().
			Str("device", device).
			Str("mountPoint", *mountPointFlag).
			Msg("Successfully mounted")

		defer syscall.Unmount(*mountPointFlag, 0)

		from, err := os.Open(*mountPointFlag + *fromFlag)
		if err != nil {
			log.Error().
				Err(err).
				Str("from", *fromFlag).
				Msg("Cannot open")
			return
		}

		defer from.Close()

		to, err := os.OpenFile(*toFlag, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Error().
				Err(err).
				Str("to", *toFlag).
				Msg("Cannot open")
			return
		}

		defer to.Close()

		_, err = io.Copy(to, from)
		if err != nil {
			log.Error().
				Str("to", *toFlag).
				Str("from", *fromFlag).
				Err(err).
				Msg("Cannot copy file content")
			return
		}

		log.Info().
			Str("to", *toFlag).
			Str("from", *fromFlag).
			Str("class", *classFlag).
			Msg("Successfully copied")
	}
}
