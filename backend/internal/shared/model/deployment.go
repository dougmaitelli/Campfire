package model

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"campfire/internal/shared/limits"
)

// Deployment contains execution instructions, independent of panel authoring models.
type Deployment struct {
	ServerID      string               `json:"server_id"`
	Configuration RuntimeConfiguration `json:"configuration"`
	Ember         RuntimeDefinition    `json:"ember"`
}

// RuntimeConfiguration contains placement, selected values, and enforced limits.
type RuntimeConfiguration struct {
	SparkID         string            `json:"spark_id"`
	EmberID         string            `json:"ember_id"`
	Image           string            `json:"image"`
	Variables       map[string]string `json:"variables"`
	CPULimit        float64           `json:"cpu_limit"`
	MemoryMB        int64             `json:"memory_mb"`
	DiskMB          int64             `json:"disk_mb"`
	Ports           []ServerPort      `json:"ports"`
	GameIntegration bool              `json:"game_integration"`
}

// RuntimeDefinition is the executable subset of an Ember, with one startup command.
type RuntimeDefinition struct {
	Startup      string       `json:"startup"`
	StartupShell string       `json:"startup_shell,omitempty"`
	WorkDir      string       `json:"work_dir"`
	User         string       `json:"user"`
	Installer    *Installer   `json:"installer,omitempty"`
	Stop         string       `json:"stop,omitempty"`
	Ready        string       `json:"ready,omitempty"`
	Readiness    *Readiness   `json:"readiness,omitempty"`
	Configs      []ConfigFile `json:"configs,omitempty"`
}

// NewDeployment projects saved panel settings into Spark execution instructions.
func NewDeployment(serverID string, c ServerConfiguration, e Ember) (Deployment, error) {
	if c.EmberID == "" || c.EmberID != e.ID {
		return Deployment{}, errors.New("Ember identity mismatch")
	}

	if err := e.Validate(); err != nil {
		return Deployment{}, err
	}

	allowed := false
	for _, image := range e.Images {
		allowed = allowed || c.Image == image
	}

	if !allowed {
		return Deployment{}, errors.New("selected image is not available in the Ember")
	}

	for _, variable := range e.Variables {
		if err := variable.ValidateValue(c.Variables[variable.Name]); err != nil {
			return Deployment{}, err
		}
	}

	_, startup, err := e.ServerStartup(c)
	if err != nil {
		return Deployment{}, err
	}

	variables := maps.Clone(c.Variables)
	if variables == nil {
		variables = make(map[string]string)
	}

	// Required ports are authoring metadata; only their fallback value is needed.
	if len(c.Ports) == 0 && len(e.RequiredPorts) > 0 {
		variables["SERVER_PORT"] = strconv.Itoa(e.RequiredPorts[0].Container)
	}

	deployment := Deployment{
		ServerID: serverID,
		Configuration: RuntimeConfiguration{
			SparkID: c.SparkID, EmberID: c.EmberID, Image: c.Image,
			Variables: variables, CPULimit: c.CPULimit, MemoryMB: c.MemoryMB,
			DiskMB: c.DiskMB, Ports: slices.Clone(c.Ports), GameIntegration: c.GameIntegration,
		},
		Ember: RuntimeDefinition{
			Startup: startup, StartupShell: e.StartupShell, WorkDir: e.WorkDir, User: e.User,
			Installer: e.Installer, Stop: e.Stop, Ready: e.Ready, Readiness: e.Readiness, Configs: e.Configs,
		},
	}

	return deployment, deployment.Validate()
}

// Validate checks execution inputs at the Spark trust boundary.
func (d Deployment) Validate() error {
	c := d.Configuration
	if c.EmberID == "" || Image(c.Image) != nil || math.IsNaN(c.CPULimit) || math.IsInf(c.CPULimit, 0) || c.CPULimit < limits.ServerMinCPU || c.CPULimit > limits.MaxCPU || c.MemoryMB < limits.MinMemoryMiB || c.MemoryMB > limits.MaxMemoryMiB || c.DiskMB < 0 || c.DiskMB > limits.MaxDiskMiB || len(c.Ports) > limits.MaxServerPorts {
		return errors.New("invalid server image or resources")
	}

	for _, port := range c.Ports {
		ip, err := netip.ParseAddr(port.BindIP)
		if err != nil || ip.Zone() != "" || ip.IsMulticast() || port.HostPort < limits.MinPort || port.HostPort > limits.MaxPort || port.ContainerPort < limits.MinPort || port.ContainerPort > limits.MaxPort || (port.Protocol != NetworkTCP && port.Protocol != NetworkUDP) {
			return errors.New("invalid server port")
		}
	}

	known := BuiltinEnvironment()

	for name, value := range c.Variables {
		if !envName.MatchString(name) || name == "STARTUP" || strings.ContainsRune(value, '\x00') {
			return errors.New("invalid server environment")
		}

		known[name] = ""
	}

	return d.Ember.validate(known)
}

// DecodeStoredDeployment decodes and validates persisted execution instructions.
func DecodeStoredDeployment(data []byte) (Deployment, error) {
	var deployment Deployment
	if err := json.Unmarshal(data, &deployment); err != nil {
		return Deployment{}, err
	}

	return deployment, deployment.Validate()
}

// ConfigurationReceipt acknowledges durable configuration storage, not installation.
type ConfigurationReceipt struct {
	ServerID string `json:"server_id"`
	Revision string `json:"revision"`
}
