package model

// PrimaryPort returns the explicitly designated container port. Absence means
// no main port; ordering and descriptions never designate one.
func (e Ember) PrimaryPort() (RequiredPort, bool) {
	for _, port := range e.RequiredPorts {
		if port.Primary {
			return port, true
		}
	}

	return RequiredPort{}, false
}

// PrimaryHostPort resolves a designated port through this server's mappings.
// Multiple mappings are usable only when they agree on the external port.
func PrimaryHostPort(ports []ServerPort, required []RequiredPort) int {
	primary, ok := (Ember{RequiredPorts: required}).PrimaryPort()
	if !ok {
		return 0
	}

	result := 0

	for _, port := range ports {
		if port.ContainerPort != primary.Container || port.Protocol != primary.Protocol {
			continue
		}

		if port.HostPort < 1 || port.HostPort > 65535 {
			return 0
		}

		if result != 0 && result != port.HostPort {
			return 0
		}

		result = port.HostPort
	}

	return result
}
