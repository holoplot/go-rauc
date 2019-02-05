# Golang bindings for RAUC

This Go package provides bindings for [RAUC](https://rauc.io).

For details on the properties and calling semantics, please refer to the
[DBus API documentation](https://rauc.readthedocs.io/en/latest/reference.html#d-bus-api)
of the RAUC project.

# Install

Install the package like this:

```
go get https://github.com/holoplot/go-rauc
```

And then use it in your source code.

```
import "github.com/holoplot/go-rauc"
```

# Example

Below is an example to illustrate the usage of this package.
Note that you will need to have a working RAUC installation, including a valid config and all.

```go
package main

import (
	"log"
	"github.com/holoplot/go-rauc"
)

func main() {
	raucInstaller, err := rauc.InstallerNew()
	if err != nil {
		log.Fatal("Cannot create RaucInstaller")
	}

	operation, err := raucInstaller.GetOperation()
	if err != nil {
		log.Fatal("GetOperation() failed")
	}
	log.Printf("Operation: %s", operation)


	bootSlot, err := raucInstaller.GetBootSlot()
	if err != nil {
		log.Fatal("GetBootSlot() failed")
	}
	log.Printf("Boot slot: %s", bootSlot)

	slotStatus, err := raucInstaller.GetSlotStatus()
	if err != nil {
		log.Fatal("GetSlotStatus() failed")
	}

	for count, status := range slotStatus {
		log.Printf("status[%d]: %s", count, status.SlotName)

		for k, v := range status.Status {
			log.Printf("    %s: %s", k, v.String())
		}
	}

	variant, err := raucInstaller.GetVariant()
	if err != nil {
		log.Fatal("GetVariant() failed")
	}
	log.Printf("Variant: %s", variant)

	percentage, message, nestingDepth, err := raucInstaller.GetProgress()
	if err != nil {
		log.Fatal("GetProgress() failed", message)
	}
	log.Printf("Progress: percentage=%d, message=%s, nestingDepth=%d", percentage, message, nestingDepth)

	filename := "/path/to/update.raucb"
	compatible, version, err := raucInstaller.Info(filename)
	if err != nil {
		log.Fatal("Info() failed", err.Error())
	}
	log.Printf("Info(): compatible=%s, version=%s", compatible, version)

	err = raucInstaller.Install(filename)
	if err != nil {
		log.Fatal("Install() failed: ", err.Error())
	}
}
```

# MIT License

See file `LICENSE` for details.

