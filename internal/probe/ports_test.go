package probe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFakeSys creates sysRoot/class/tty/<dev>/device/{idVendor,idProduct}
// under root so ScanPorts can resolve VID:PID. Multiple calls with the same
// root add more devices; call buildFakeSysRoot first to get a shared root.
func buildFakeSysRoot(t *testing.T) (root, sysRoot, devRoot string) {
	t.Helper()
	root = t.TempDir()
	sysRoot = filepath.Join(root, "sys")
	devRoot = filepath.Join(root, "dev")
	os.MkdirAll(devRoot, 0o755)
	return root, sysRoot, devRoot
}

func addFakeDevice(t *testing.T, sysRoot, devRoot, dev, vid, pid string) {
	t.Helper()
	devDir := filepath.Join(sysRoot, "class", "tty", dev, "device")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "idVendor"), []byte(vid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "idProduct"), []byte(pid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devRoot, dev), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildFakeSys(t *testing.T, dev, vid, pid string) (sysRoot, devRoot string) {
	t.Helper()
	_, sysRoot, devRoot = buildFakeSysRoot(t)
	addFakeDevice(t, sysRoot, devRoot, dev, vid, pid)
	return sysRoot, devRoot
}

func TestScanPorts_SkipsIncompleteVidPid(t *testing.T) {
	_, sysRoot, devRoot := buildFakeSysRoot(t)
	// Write only idVendor — idProduct is intentionally absent.
	devDir := filepath.Join(sysRoot, "class", "tty", "ttyXRUSB0", "device")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "idVendor"), []byte("04e2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devRoot, "ttyXRUSB0"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	ports, err := ScanPorts(sysRoot, devRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 0 {
		t.Fatalf("expected 0 ports for device missing idProduct, got %d: %+v", len(ports), ports)
	}
}

func TestScanPorts_FindsXRUSB(t *testing.T) {
	sysRoot, devRoot := buildFakeSys(t, "ttyXRUSB0", "04e2", "1411")
	ports, err := ScanPorts(sysRoot, devRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0].Vid != "04e2" || ports[0].Pid != "1411" {
		t.Fatalf("ports: %+v", ports)
	}
	if ports[0].Dev != filepath.Join(devRoot, "ttyXRUSB0") {
		t.Fatalf("unexpected Dev path: %q", ports[0].Dev)
	}
}

func TestScanPorts_FindsXRUSBAndACM(t *testing.T) {
	_, sysRoot, devRoot := buildFakeSysRoot(t)
	addFakeDevice(t, sysRoot, devRoot, "ttyXRUSB0", "04e2", "1411")
	addFakeDevice(t, sysRoot, devRoot, "ttyACM0", "04e2", "1411")
	ports, err := ScanPorts(sysRoot, devRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d: %+v", len(ports), ports)
	}
	devs := map[string]bool{}
	for _, p := range ports {
		devs[p.Dev] = true
		if p.Vid != "04e2" || p.Pid != "1411" {
			t.Fatalf("unexpected vid/pid in port %+v", p)
		}
	}
	if !devs[filepath.Join(devRoot, "ttyXRUSB0")] {
		t.Fatalf("ttyXRUSB0 not found in ports: %+v", ports)
	}
	if !devs[filepath.Join(devRoot, "ttyACM0")] {
		t.Fatalf("ttyACM0 not found in ports: %+v", ports)
	}
}

func TestScanPorts_FindsTTYUSB(t *testing.T) {
	// On a modern kernel the in-tree xr_serial driver binds the Exar XR21B1411
	// and exposes it as /dev/ttyUSB0 (confirmed on the .244 server, kernel 6.8).
	sysRoot, devRoot := buildFakeSys(t, "ttyUSB0", "04e2", "1411")
	ports, err := ScanPorts(sysRoot, devRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0].Vid != "04e2" || ports[0].Pid != "1411" {
		t.Fatalf("ports: %+v", ports)
	}
	if ports[0].Dev != filepath.Join(devRoot, "ttyUSB0") {
		t.Fatalf("unexpected Dev path: %q", ports[0].Dev)
	}
}

// addRealisticUSBDevice mirrors the real sysfs layout: the tty's `device`
// symlink points at the USB *interface* (e.g. 1-3:1.0), while idVendor/idProduct
// live on the parent USB *device* (1-3), one level up. Confirmed on .244.
func addRealisticUSBDevice(t *testing.T, sysRoot, devRoot, dev, vid, pid string) {
	t.Helper()
	usbDev := filepath.Join(sysRoot, "devices", "usb1", "1-3")
	iface := filepath.Join(usbDev, "1-3:1.0")
	if err := os.MkdirAll(iface, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usbDev, "idVendor"), []byte(vid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usbDev, "idProduct"), []byte(pid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ttyDir := filepath.Join(sysRoot, "class", "tty", dev)
	if err := os.MkdirAll(ttyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(iface, filepath.Join(ttyDir, "device")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devRoot, dev), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanPorts_RealisticSysfsLayout(t *testing.T) {
	_, sysRoot, devRoot := buildFakeSysRoot(t)
	addRealisticUSBDevice(t, sysRoot, devRoot, "ttyUSB0", "04e2", "1411")
	ports, err := ScanPorts(sysRoot, devRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0].Vid != "04e2" || ports[0].Pid != "1411" {
		t.Fatalf("ports: %+v", ports)
	}
	if ports[0].Dev != filepath.Join(devRoot, "ttyUSB0") {
		t.Fatalf("unexpected Dev path: %q", ports[0].Dev)
	}
}

func TestNeedsExarDriver_ACM(t *testing.T) {
	if !NeedsExarDriver([]Port{{Dev: "/dev/ttyACM0", Vid: "04e2", Pid: "1411"}}) {
		t.Fatal("expected driver needed for ttyACM0 + 04e2")
	}
	if NeedsExarDriver([]Port{{Dev: "/dev/ttyXRUSB0", Vid: "04e2"}}) {
		t.Fatal("ttyXRUSB means driver already loaded")
	}
}

func TestRenderUdevRule_KeysOnIDPath(t *testing.T) {
	r := RenderUdevRule(Port{Vid: "04e2", Pid: "1411", IDPath: "pci-0000:00:14.0-usb-0:1:1.0"}, "solar-inv1")
	if r == "" || !strings.Contains(r, "ID_PATH") || !strings.Contains(r, "solar-inv1") {
		t.Fatalf("rule: %q", r)
	}
	if !strings.Contains(r, "pci-0000:00:14.0-usb-0:1:1.0") {
		t.Fatalf("real IDPath not present in rule: %q", r)
	}
}

func TestRenderUdevRule_EmptyIDPathShowsPlaceholder(t *testing.T) {
	r := RenderUdevRule(Port{Vid: "04e2", Pid: "1411"}, "solar-inv1")
	if !strings.Contains(r, "FILL:") {
		t.Fatalf("expected FILL: placeholder in rule: %q", r)
	}
	if !strings.Contains(r, "udevadm") {
		t.Fatalf("expected udevadm in rule placeholder: %q", r)
	}
}
