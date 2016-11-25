package container

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	libvirtgo "github.com/rgbkrk/libvirt-go"
)

var connectionAddress = "qemu:///system"

type vmBaseConfig struct {
	numCPU           int
	DefaultMaxCpus   int
	DefaultMaxMem    int
	Memory           int
	OriginalDiskPath string
}

type memory struct {
	Unit    string `xml:"unit,attr"`
	Content int    `xml:",chardata"`
}

type maxmem struct {
	Unit    string `xml:"unit,attr"`
	Slots   string `xml:"slots,attr"`
	Content int    `xml:",chardata"`
}

type vcpu struct {
	Placement string `xml:"placement,attr"`
	Current   string `xml:"current,attr"`
	Content   int    `xml:",chardata"`
}

type cell struct {
	Id     string `xml:"id,attr"`
	Cpus   string `xml:"cpus,attr"`
	Memory string `xml:"memory,attr"`
	Unit   string `xml:"unit,attr"`
}

type cpu struct {
	Mode string `xml:"mode,attr"`
}

type ostype struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Content string `xml:",chardata"`
}

type domainos struct {
	Supported string `xml:"supported,attr"`
	Type      ostype `xml:"type"`
}

type feature struct {
	Acpi acpi `xml:"acpi"`
}

type acpi struct {
}

type fspath struct {
	Dir string `xml:"dir,attr"`
}

type filesystem struct {
	Type       string `xml:"type,attr"`
	Accessmode string `xml:"accessmode,attr"`
	Source     fspath `xml:"source"`
	Target     fspath `xml:"target"`
}

type diskdriver struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
}

type disksource struct {
	File string `xml:"file,attr"`
}

type diskformat struct {
	Type string `xml:"type,attr"`
}

type backingstore struct {
	Type   string     `xml:"type,attr"`
	Index  string     `xml:"index,attr"`
	Format diskformat `xml:"format"`
	Source disksource `xml:"source"`
}

type disktarget struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

type readonly struct {
}

type controller struct {
	Type  string `xml:"type,attr"`
	Model string `xml:"model,attr"`
}

type disk struct {
	Type         string        `xml:"type,attr"`
	Device       string        `xml:"device,attr"`
	Driver       diskdriver    `xml:"driver"`
	Source       disksource    `xml:"source"`
	BackingStore *backingstore `xml:"backingstore,omitempty"`
	Target       disktarget    `xml:"target"`
	Readonly     *readonly     `xml:"readonly,omitempty"`
}

type channsrc struct {
	Mode string `xml:"mode,attr"`
	Path string `xml:"path,attr"`
}

type constgt struct {
	Type string `xml:"type,attr,omitempty"`
	Port string `xml:"port,attr"`
}

type console struct {
	Type   string   `xml:"type,attr"`
	Source channsrc `xml:"source"`
	Target constgt  `xml:"target"`
}

type device struct {
	Emulator          string       `xml:"emulator"`
	Filesystems       []filesystem `xml:"filesystem"`
	Disks             []disk       `xml:"disk"`
	Consoles          []console    `xml:"console"`
	NetworkInterfaces []nic        `xml:"interface"`
	Controller        []controller `xml:"controller"`
}

type seclab struct {
	Type string `xml:"type,attr"`
}

type domain struct {
	XMLName    xml.Name  `xml:"domain"`
	Type       string    `xml:"type,attr"`
	Name       string    `xml:"name"`
	Memory     memory    `xml:"memory"`
	MaxMem     *maxmem   `xml:"maxMemory,omitempty"`
	VCpu       vcpu      `xml:"vcpu"`
	OS         domainos  `xml:"os"`
	Features   []feature `xml:"features"`
	CPU        cpu       `xml:"cpu"`
	OnPowerOff string    `xml:"on_poweroff"`
	OnReboot   string    `xml:"on_reboot"`
	OnCrash    string    `xml:"on_crash"`
	Devices    device    `xml:"devices"`
	SecLabel   seclab    `xml:"seclabel"`
}

type nicmac struct {
	Address string `xml:"address,attr"`
}

type nicsrc struct {
	Bridge string `xml:"bridge,attr"`
}

type nicmodel struct {
	Type string `xml:"type,attr"`
}

type nic struct {
	Type   string   `xml:"type,attr"`
	Mac    nicmac   `xml:"mac"`
	Source nicsrc   `xml:"source"`
	Model  nicmodel `xml:"model"`
}

func (container *Container) InitDriver() *LibvirtDriver {
	conn, err := libvirtgo.NewVirConnection(connectionAddress)
	if err != nil {
		logrus.Error("failed to connect to libvirt daemon ", connectionAddress, err)
		return nil
	}

	return &LibvirtDriver{
		conn: conn,
	}
}

func (ld *LibvirtDriver) InitContext(c *Container) *LibvirtContext {
	return &LibvirtContext{
		driver:    ld,
		container: c,
	}
}

func (lc *LibvirtContext) CreateSeedImage(seedDirectory string) (string, error) {
	getisoimagePath, err := exec.LookPath("genisoimage")
	if err != nil {
		return "", fmt.Errorf("genisoimage is not installed on your PATH. Please, install it to run isolated container")
	}

	// Create user-data to be included in seed.img
	userDataString := `#cloud-config
runcmd:
 - mount -t 9p -o trans=virtio share_dir /mnt
 - chroot /mnt %s > /dev/hvc1 2>&1
 - init 0
`

	metaDataString := `#cloud-config
network-interfaces: |
  auto eth0
  iface eth0 inet static
  address %s
  netmask %s
  gateway %s
`

	var command string
	if len(lc.container.Args) > 0 {
		args := []string{}
		for _, arg := range lc.container.Args {
			if strings.Contains(arg, " ") {
				args = append(args, fmt.Sprintf("'%s'", arg))
			} else {
				args = append(args, arg)
			}
		}
		argsAsString := strings.Join(args, " ")

		command = fmt.Sprintf("%s %s", lc.container.Path, argsAsString)
	} else {
		command = lc.container.Path
	}

	// TODO - move this to a separate method
	cidrIP := lc.container.NetworkSettings.Networks["bridge"].IPAddress + "/" + strconv.Itoa(lc.container.NetworkSettings.Networks["bridge"].IPPrefixLen)
	_, IPnet, err := net.ParseCIDR(cidrIP)
	if err != nil {
		return "", fmt.Errorf("Could not parse CIDR")
	}

	netMask := strconv.Itoa(int(IPnet.Mask[0])) + "." + strconv.Itoa(int(IPnet.Mask[1])) + "." + strconv.Itoa(int(IPnet.Mask[2])) + "." + strconv.Itoa(int(IPnet.Mask[3]))

	logrus.Debugf("The user data is: %s", fmt.Sprintf(userDataString, command))
	logrus.Debugf("The meta data is: %s", fmt.Sprintf(metaDataString, lc.container.NetworkSettings.Networks["bridge"].IPAddress, netMask, lc.container.NetworkSettings.Networks["bridge"].Gateway))

	userData := []byte(fmt.Sprintf(userDataString, command))
	metaData := []byte(fmt.Sprintf(metaDataString, lc.container.NetworkSettings.Networks["bridge"].IPAddress, netMask, lc.container.NetworkSettings.Networks["bridge"].Gateway))

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Could not determine the current directory")
	}

	err = os.Chdir(seedDirectory)
	if err != nil {
		return "", fmt.Errorf("Could not changed to directory %s", seedDirectory)
	}

	writeErrorUserData := ioutil.WriteFile("user-data", userData, 0700)
	if writeErrorUserData != nil {
		return "", fmt.Errorf("Could not write user-data to /var/run/docker-qemu/%s", lc.container.ID)
	}

	writeErrorMetaData := ioutil.WriteFile("meta-data", metaData, 0700)
	if writeErrorMetaData != nil {
		return "", fmt.Errorf("Could not write meta-data to /var/run/docker-qemu/%s", lc.container.ID)
	}

	logrus.Debugf("genisoimage path: %s", getisoimagePath)

	err = exec.Command(getisoimagePath, "-output", "seed.img", "-volid", "cidata", "-joliet", "-rock", "user-data", "meta-data").Run()
	if err != nil {
		return "", fmt.Errorf("Could not execute genisoimage")
	}

	err = os.Chdir(currentDir)
	if err != nil {
		return "", fmt.Errorf("Could not changed to directory %s", currentDir)
	}

	return seedDirectory + "/seed.img", nil
}

func (lc *LibvirtContext) CreateDeltaDiskImage(deltaDiskDirectory, diskPath string) (string, error) {
	deltaImagePath, err := exec.LookPath("qemu-img")
	if err != nil {
		return "", fmt.Errorf("qemu-img is not installed on your PATH. Please, install it to run isolated qemu container")
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Could not determine the current directory")
	}

	err = os.Chdir(deltaDiskDirectory)
	if err != nil {
		return "", fmt.Errorf("Could not changed to directory %s", deltaDiskDirectory)
	}

	err = exec.Command(deltaImagePath, "create", "-f", "qcow2", "-b", diskPath, "disk.img").Run()
	if err != nil {
		return "", fmt.Errorf("Could not execute qemu-img")
	}

	err = os.Chdir(currentDir)
	if err != nil {
		return "", fmt.Errorf("Could not changed to directory %s", currentDir)
	}

	return deltaDiskDirectory + "/disk.img", nil
}

func (lc *LibvirtContext) DomainXml() (string, error) {
	baseCfg := &vmBaseConfig{
		numCPU:           1,
		DefaultMaxCpus:   2,
		DefaultMaxMem:    256,
		Memory:           256,
		OriginalDiskPath: "/var/lib/libvirt/images/disk.img.orig",
	}

	// Create directory for seed image and delta disk image
	directory := lc.container.Config.QemuDirectory

	deltaDiskImageLocation, err := lc.CreateDeltaDiskImage(directory, baseCfg.OriginalDiskPath)
	if err != nil {
		return "", fmt.Errorf("Could not create delta disk image")
	}

	logrus.Debugf("Delta disk image location: %s", deltaDiskImageLocation)

	// Domain XML Formation
	dom := &domain{
		Type: "kvm",
		Name: lc.container.ID[0:12],
	}

	dom.Memory.Unit = "MiB"
	dom.Memory.Content = baseCfg.Memory

	dom.VCpu.Current = strconv.Itoa(baseCfg.numCPU)
	dom.VCpu.Content = baseCfg.numCPU

	dom.OS.Supported = "yes"
	dom.OS.Type.Content = "hvm"

	acpiFeature := feature{
		Acpi: acpi{},
	}
	dom.Features = append(dom.Features, acpiFeature)

	dom.SecLabel.Type = "none"

	dom.CPU.Mode = "host-model"

	dom.OnPowerOff = "destroy"
	dom.OnReboot = "destroy"
	dom.OnCrash = "destroy"

	diskimage := disk{
		Type:   "file",
		Device: "disk",
		Driver: diskdriver{
			Name: "qemu",
			Type: "qcow2",
		},
		Source: disksource{
			File: deltaDiskImageLocation,
		},
		BackingStore: &backingstore{
			Type:  "file",
			Index: "1",
			Format: diskformat{
				Type: "raw",
			},
			Source: disksource{
				File: baseCfg.OriginalDiskPath,
			},
		},
		Target: disktarget{
			Dev: "sda",
			Bus: "scsi",
		},
	}
	dom.Devices.Disks = append(dom.Devices.Disks, diskimage)

	seedimage := disk{
		Type:   "file",
		Device: "cdrom",
		Driver: diskdriver{
			Name: "qemu",
			Type: "raw",
		},
		Source: disksource{
			File: fmt.Sprintf("%s/seed.img", lc.container.Config.QemuDirectory),
		},
		Target: disktarget{
			Dev: "sdb",
			Bus: "scsi",
		},
		Readonly: &readonly{},
	}
	dom.Devices.Disks = append(dom.Devices.Disks, seedimage)

	storageController := controller{
		Type:  "scsi",
		Model: "virtio-scsi",
	}
	dom.Devices.Controller = append(dom.Devices.Controller, storageController)

	macAddress := lc.container.CommonContainer.NetworkSettings.Networks["bridge"].MacAddress
	networkInterface := nic{
		Type: "bridge",
		Mac: nicmac{
			Address: macAddress,
		},
		Source: nicsrc{
			Bridge: "docker0",
		},
		Model: nicmodel{
			Type: "virtio",
		},
	}
	dom.Devices.NetworkInterfaces = append(dom.Devices.NetworkInterfaces, networkInterface)

	fs := filesystem{
		Type:       "mount",
		Accessmode: "passthrough",
		Source: fspath{
			Dir: lc.container.BaseFS,
		},
		Target: fspath{
			Dir: "share_dir",
		},
	}
	dom.Devices.Filesystems = append(dom.Devices.Filesystems, fs)

	serialConsole := console{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: fmt.Sprintf("%s/serial.sock", lc.container.Config.QemuDirectory),
		},
		Target: constgt{
			Type: "serial",
			Port: "0",
		},
	}
	dom.Devices.Consoles = append(dom.Devices.Consoles, serialConsole)
	logrus.Debugf("Serial console socket location: %s", fmt.Sprintf("%s/serial.sock", lc.container.Config.QemuDirectory))

	vmConsole := console{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: fmt.Sprintf("%s/arbritary.sock", lc.container.Config.QemuDirectory),
		},
		Target: constgt{
			Type: "virtio",
			Port: "1",
		},
	}
	dom.Devices.Consoles = append(dom.Devices.Consoles, vmConsole)

	appConsole := console{
		Type: "unix",
		Source: channsrc{
			Mode: "bind",
			Path: fmt.Sprintf("%s/app.sock", lc.container.Config.QemuDirectory),
		},
		Target: constgt{
			Type: "virtio",
			Port: "2",
		},
	}
	dom.Devices.Consoles = append(dom.Devices.Consoles, appConsole)
	logrus.Debugf("Application console socket location: %s", fmt.Sprintf("%s/app.sock", lc.container.Config.QemuDirectory))

	data, err := xml.Marshal(dom)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (lc *LibvirtContext) GetDomain() *libvirtgo.VirDomain {
	return lc.domain
}

func (lc *LibvirtContext) GetQemuDirectory() string {
	return lc.container.Config.QemuDirectory
}

func (lc *LibvirtContext) CreateDomain() {
	domainXml, err := lc.DomainXml()
	if err != nil {
		logrus.Error("Fail to get domain xml configuration:", err)
		return
	}
	logrus.Debugf("domainXML: %v", domainXml)

	var domain libvirtgo.VirDomain
	domain, err = lc.driver.conn.DomainDefineXML(domainXml)
	if err != nil {
		logrus.Error("Failed to launch domain ", err)
		return
	}

	lc.domain = &domain

}

func (lc *LibvirtContext) Launch() {
	if lc.domain == nil {
		logrus.Error("Failed to launch domain as no domain in LibvirtContext")
		return
	}

	err := lc.domain.Create()
	if err != nil {
		logrus.Error("Fail to start qemu isolated container ", err)
		return
	}

	logrus.Infof("Domain has started: %v", lc.container.ID)
}

func (lc *LibvirtContext) Shutdown() {
	if lc.domain == nil {
		return
	}

	lc.domain.DestroyFlags(libvirtgo.VIR_DOMAIN_DESTROY_DEFAULT)
	logrus.Infof("Domain has shutdown: %v", lc.container.ID)
}

func (lc *LibvirtContext) Undefine() {
	if lc.domain == nil {
		return
	}
	err := lc.domain.Undefine()
	if err == nil {
		logrus.Infof("Domain is undefined: %v", lc.container.ID)
	} else {
		logrus.Infof("Failed to undefine domain: %v", lc.container.ID)
	}
}

func (lc *LibvirtContext) Close() {
	lc.domain = nil
}

func (lc *LibvirtContext) Pause(pause bool) error {
	if lc.domain == nil {
		return fmt.Errorf("Cannot find domain")
	}

	if pause {
		logrus.Infof("Domain suspended:", lc.domain.Suspend())
		return nil
	} else {
		logrus.Infof("Domain resumed:", lc.domain.Resume())
		return nil
	}
}
