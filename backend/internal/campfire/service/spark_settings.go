package service

import (
	"fmt"
	"math"
	"net/netip"
	"strings"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

func validateSettings(settings *model.SparkSettings) error {
	address := strings.TrimSpace(settings.GameAddress)
	if address != "" {
		if ip, err := netip.ParseAddr(address); err == nil && ip.Zone() == "" {
			address = ip.Unmap().String()
		} else if !utils.IsValidHostname(address) {
			return fmt.Errorf("%w: game address must be an IP or hostname without a scheme, path, or port", ErrInvalidSettings)
		} else {
			address = strings.ToLower(address)
		}
	}

	settings.GameAddress = address

	if math.IsNaN(settings.CPULimit) || math.IsInf(settings.CPULimit, 0) || settings.CPULimit < 0 || settings.CPULimit > 1024 ||
		settings.MemoryMB < 0 || settings.MemoryMB > 9007199254740991 || settings.DiskMB < 0 || settings.DiskMB > 9007199254740991 {
		return fmt.Errorf("%w: CPU must be 0–1024 cores; memory and disk must be nonnegative whole MiB", ErrInvalidSettings)
	}

	if settings.PortAllocations == nil {
		settings.PortAllocations = make([]model.PortAllocation, 0)
	}

	if len(settings.PortAllocations) > 128 {
		return fmt.Errorf("%w: at most 128 port allocation ranges are allowed", ErrInvalidSettings)
	}

	for i := range settings.PortAllocations {
		allocation := &settings.PortAllocations[i]

		ip, err := netip.ParseAddr(strings.TrimSpace(allocation.BindIP))
		if err != nil || ip.Zone() != "" || ip.IsMulticast() {
			return fmt.Errorf("%w: allocation bind address must be an IPv4 or IPv6 address", ErrInvalidSettings)
		}

		ip = ip.Unmap()

		allocation.BindIP = ip.String()
		if allocation.Start < 1 || allocation.End > 65535 || allocation.End < allocation.Start ||
			(allocation.Protocol != "tcp" && allocation.Protocol != "udp") {
			return fmt.Errorf("%w: allocation ports must be 1–65535 with start ≤ end, using TCP or UDP", ErrInvalidSettings)
		}

		for _, previous := range settings.PortAllocations[:i] {
			previousIP, _ := netip.ParseAddr(previous.BindIP)

			sameBind := previousIP == ip || (previousIP.BitLen() == ip.BitLen() && (previousIP.IsUnspecified() || ip.IsUnspecified()))
			if sameBind && previous.Protocol == allocation.Protocol && previous.Start <= allocation.End && allocation.Start <= previous.End {
				return fmt.Errorf("%w: allocation ranges overlap on the same interface and protocol", ErrInvalidSettings)
			}
		}
	}

	return nil
}
func validateChanges(input *model.SparkChanges) error {
	settings := model.SparkSettings{}
	if input.GameAddress != nil {
		settings.GameAddress = *input.GameAddress
	}

	if input.CPULimit != nil {
		settings.CPULimit = *input.CPULimit
	}

	if input.MemoryMB != nil {
		settings.MemoryMB = *input.MemoryMB
	}

	if input.DiskMB != nil {
		settings.DiskMB = *input.DiskMB
	}

	if input.PortAllocations != nil {
		settings.PortAllocations = append([]model.PortAllocation{}, (*input.PortAllocations)...)
	}

	if err := validateSettings(&settings); err != nil {
		return err
	}

	if input.GameAddress != nil {
		input.GameAddress = &settings.GameAddress
	}

	if input.PortAllocations != nil {
		input.PortAllocations = &settings.PortAllocations
	}

	return nil
}
