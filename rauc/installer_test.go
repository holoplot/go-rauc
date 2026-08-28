package rauc

import (
	"testing"

	dbus "github.com/godbus/dbus/v5"
)

// Variant.String() renders the D-Bus representation of a value rather than the
// value itself, so for a string it is quoted. Reading properties with it
// returns `"owa5x-nand"` instead of `owa5x-nand`, which silently breaks any
// comparison against a bundle's compatible string. This guards the reason
// stringProperty exists.
func TestVariantStringIsNotTheUnderlyingValue(t *testing.T) {
	const want = "owa5x-nand"
	v := dbus.MakeVariant(want)

	if got := v.String(); got == want {
		t.Fatalf("Variant.String() returned %q; if this now matches the value, "+
			"stringProperty may no longer be necessary", got)
	}

	got, ok := v.Value().(string)
	if !ok {
		t.Fatalf("Variant.Value() is %T, want string", v.Value())
	}
	if got != want {
		t.Errorf("Variant.Value() = %q, want %q", got, want)
	}
}

func TestInstallBundleOptionsArgs(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		args := InstallBundleOptions{}.args()

		if got, ok := args["ignore-compatible"]; !ok || got != false {
			t.Errorf(`args["ignore-compatible"] = %v, %v; want false, true`, got, ok)
		}
	})

	t.Run("ignore incompatible", func(t *testing.T) {
		args := InstallBundleOptions{IgnoreIncompatible: true}.args()

		if got := args["ignore-compatible"]; got != true {
			t.Errorf(`args["ignore-compatible"] = %v, want true`, got)
		}
	})

	t.Run("unset options are omitted", func(t *testing.T) {
		args := InstallBundleOptions{}.args()

		for _, key := range []string{
			"ignore-version-limit", "transaction-id", "require-manifest-hash",
			"tls-cert", "tls-key", "tls-ca", "tls-no-verify", "http-headers",
		} {
			if _, ok := args[key]; ok {
				t.Errorf("args[%q] is set but no option asked for it", key)
			}
		}
	})

	t.Run("all options", func(t *testing.T) {
		args := InstallBundleOptions{
			IgnoreIncompatible:  true,
			IgnoreVersionLimit:  true,
			TransactionID:       "5f1f5e1a-8f6a-4a1e-9a4e-1b3c5d7e9f01",
			RequireManifestHash: "cafebabe",
			BundleAccessOptions: BundleAccessOptions{
				TLSCert:     "/etc/rauc/client.pem",
				TLSKey:      "pkcs11:token=rauc;object=client",
				TLSCA:       "/etc/rauc/ca.pem",
				TLSNoVerify: true,
				HTTPHeaders: []string{"Authorization: Bearer token"},
			},
		}.args()

		want := map[string]any{
			"ignore-compatible":     true,
			"ignore-version-limit":  true,
			"transaction-id":        "5f1f5e1a-8f6a-4a1e-9a4e-1b3c5d7e9f01",
			"require-manifest-hash": "cafebabe",
			"tls-cert":              "/etc/rauc/client.pem",
			"tls-key":               "pkcs11:token=rauc;object=client",
			"tls-ca":                "/etc/rauc/ca.pem",
			"tls-no-verify":         true,
		}
		for key, value := range want {
			if got := args[key]; got != value {
				t.Errorf("args[%q] = %v, want %v", key, got, value)
			}
		}

		headers, ok := args["http-headers"].([]string)
		if !ok || len(headers) != 1 || headers[0] != "Authorization: Bearer token" {
			t.Errorf(`args["http-headers"] = %v, want ["Authorization: Bearer token"]`, args["http-headers"])
		}
	})
}

func TestInspectBundleOptionsArgs(t *testing.T) {
	t.Run("defaults are empty", func(t *testing.T) {
		if args := (InspectBundleOptions{}).args(); len(args) != 0 {
			t.Errorf("args = %v, want empty", args)
		}
	})

	t.Run("access options are passed through", func(t *testing.T) {
		args := InspectBundleOptions{
			BundleAccessOptions: BundleAccessOptions{
				TLSCA:       "/etc/rauc/ca.pem",
				TLSNoVerify: true,
			},
		}.args()

		if got := args["tls-ca"]; got != "/etc/rauc/ca.pem" {
			t.Errorf(`args["tls-ca"] = %v, want "/etc/rauc/ca.pem"`, got)
		}
		if got := args["tls-no-verify"]; got != true {
			t.Errorf(`args["tls-no-verify"] = %v, want true`, got)
		}
	})
}

func propertiesChangedSignal(interfaceName string, changed map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Name: propertiesInterface + ".PropertiesChanged",
		Body: []any{interfaceName, changed, []string{}},
	}
}

func TestProgressFromPropertiesChanged(t *testing.T) {
	installerInterface := dbusInterface + ".Installer"
	want := Progress{Percentage: 42, Message: "installing", NestingDepth: 1}

	t.Run("extracts progress", func(t *testing.T) {
		signal := propertiesChangedSignal(installerInterface, map[string]dbus.Variant{
			"Progress": dbus.MakeVariant(want),
		})

		got, found := progressFromPropertiesChanged(signal)
		if !found {
			t.Fatal("progress not found in signal")
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("ignores other interfaces", func(t *testing.T) {
		signal := propertiesChangedSignal("org.example.Other", map[string]dbus.Variant{
			"Progress": dbus.MakeVariant(want),
		})

		if _, found := progressFromPropertiesChanged(signal); found {
			t.Error("accepted a signal from an unrelated interface")
		}
	})

	t.Run("ignores other properties", func(t *testing.T) {
		signal := propertiesChangedSignal(installerInterface, map[string]dbus.Variant{
			"Operation": dbus.MakeVariant("installing"),
		})

		if _, found := progressFromPropertiesChanged(signal); found {
			t.Error("reported progress from a signal that carried none")
		}
	})

	t.Run("ignores unrelated signals", func(t *testing.T) {
		signal := &dbus.Signal{
			Name: dbusInterface + ".Installer.Completed",
			Body: []any{int32(0)},
		}

		if _, found := progressFromPropertiesChanged(signal); found {
			t.Error("accepted a Completed signal as progress")
		}
	})

	t.Run("tolerates nil", func(t *testing.T) {
		if _, found := progressFromPropertiesChanged(nil); found {
			t.Error("accepted a nil signal")
		}
	})
}
