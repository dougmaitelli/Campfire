package runtime

import (
	_ "embed"
	"encoding/json"
	"math/bits"
	"path/filepath"
)

// Docker's default seccomp profile, Apache-2.0, from moby/profiles (2026-10-14).
// Retain its capability and architecture rules, narrowing only filesystem
// attribute writes that could remove project IDs or their inheritance flags.
//
//go:embed seccomp/default.json
var defaultSeccomp []byte

func quotaSeccomp() ([]byte, error) {
	var profile struct {
		DefaultAction   string           `json:"defaultAction"`
		DefaultErrnoRet int              `json:"defaultErrnoRet"`
		ArchMap         json.RawMessage  `json:"archMap"`
		Syscalls        []map[string]any `json:"syscalls"`
	}
	if err := json.Unmarshal(defaultSeccomp, &profile); err != nil {
		return nil, err
	}

	for _, rule := range profile.Syscalls {
		names, _ := rule["names"].([]any)

		filtered := make([]any, 0, len(names))
		for _, name := range names {
			if name != "ioctl" {
				filtered = append(filtered, name)
			}
		}

		rule["names"] = filtered
	}
	// The syscall argument is unsigned int; mask its low 32 bits to cover
	// sign-extended requests from native and compatibility ABIs alike.
	// Forbid FSSETXATTR and SETFLAGS (32/64 bit). The newer file_setattr
	// syscall is already excluded by the default allowlist.
	blocked := []uint64{0x401c5820, 0x40086602, 0x40046602}
	// A broad ALLOW rule subsumes narrower rules in libseccomp. Express the
	// complement as disjoint allowed bit prefixes, leaving the forbidden
	// requests to the default EPERM action. Each rule tests one argument once.
	var allow func(uint64, uint64, []uint64)

	allow = func(prefix, mask uint64, denied []uint64) {
		if len(denied) == 0 {
			profile.Syscalls = append(profile.Syscalls, map[string]any{
				"names": []string{"ioctl"}, "action": "SCMP_ACT_ALLOW",
				"args": []map[string]any{{"index": 1, "value": mask, "valueTwo": prefix, "op": "SCMP_CMP_MASKED_EQ"}},
			})

			return
		}

		if mask == 0xffffffff {
			return
		}

		bit := uint64(1) << (31 - bits.OnesCount64(mask))
		for _, child := range []uint64{prefix, prefix | bit} {
			var matching []uint64

			for _, request := range denied {
				if request&(mask|bit) == child {
					matching = append(matching, request)
				}
			}

			allow(child, mask|bit, matching)
		}
	}
	allow(0, 0, blocked)

	return json.Marshal(profile)
}

func (m *Manager) seccompPath() (string, error) {
	data, err := quotaSeccomp()
	if err != nil {
		return "", err
	}

	name, err := filepath.Abs(filepath.Join(m.Directory, ".quota-seccomp.json"))
	if err != nil {
		return "", err
	}
	// Each operation publishes an identical profile atomically. Docker reads
	// the profile into its container configuration, so no daemon-side path is needed.
	return name, writeAtomic(name, data, 0600)
}
