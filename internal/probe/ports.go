package probe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Port struct {
	Dev    string
	Vid    string
	Pid    string
	IDPath string
}

func ScanPorts(sysRoot, devRoot string) ([]Port, error) {
	ttyDir := filepath.Join(sysRoot, "class", "tty")
	entries, err := os.ReadDir(ttyDir)
	if err != nil {
		return nil, err
	}
	var ports []Port
	for _, e := range entries {
		name := e.Name()
		// ttyXRUSB*: out-of-tree Exar driver. ttyACM*: cdc_acm grabbed it
		// (needs the xr driver/blacklist). ttyUSB*: in-tree xr_serial driver
		// (modern kernels) or an external USB-RS485 adapter — the device's
		// VID:PID disambiguates which. All three can carry the inverter.
		if !strings.HasPrefix(name, "ttyXRUSB") &&
			!strings.HasPrefix(name, "ttyACM") &&
			!strings.HasPrefix(name, "ttyUSB") {
			continue
		}
		vid, pid := readUSBIDs(filepath.Join(ttyDir, name, "device"))
		if vid == "" || pid == "" {
			continue
		}
		ports = append(ports, Port{
			Dev: filepath.Join(devRoot, name),
			Vid: vid,
			Pid: pid,
		})
	}
	return ports, nil
}

// readUSBIDs resolves a tty's `device` symlink and walks up the parent
// directories to find idVendor/idProduct. On real sysfs the `device` link
// points at the USB interface (e.g. 1-3:1.0); the IDs live on the parent USB
// device (1-3), one level up. The walk also handles the degenerate case where
// the IDs sit directly in the resolved directory.
func readUSBIDs(deviceLink string) (vid, pid string) {
	dir, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return "", ""
	}
	for i := 0; i < 8; i++ {
		vid = readTrim(filepath.Join(dir, "idVendor"))
		pid = readTrim(filepath.Join(dir, "idProduct"))
		if vid != "" && pid != "" {
			return vid, pid
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", ""
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func NeedsExarDriver(ports []Port) bool {
	for _, p := range ports {
		if strings.Contains(p.Dev, "ttyACM") && p.Vid == "04e2" {
			return true
		}
	}
	return false
}

func RenderUdevRule(p Port, symlink string) string {
	idPath := p.IDPath
	if idPath == "" {
		idPath = "<FILL: udevadm info -q property -n DEV | grep ID_PATH>"
	}
	return fmt.Sprintf(
		`SUBSYSTEM=="tty", ATTRS{idVendor}=="%s", ATTRS{idProduct}=="%s", ENV{ID_PATH}=="%s", SYMLINK+="%s"`,
		p.Vid, p.Pid, idPath, symlink,
	)
}
