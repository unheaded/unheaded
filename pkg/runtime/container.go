// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"encoding/json"
)

// OCISpec represents an OCI runtime specification.
type OCISpec struct {
	OCIVersion  string            `json:"ociVersion"`
	Process     *OCIProcess       `json:"process"`
	Root        *OCIRoot          `json:"root"`
	Hostname    string            `json:"hostname,omitempty"`
	Mounts      []OCIMount        `json:"mounts,omitempty"`
	Linux       *OCILinux         `json:"linux,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OCIProcess represents the OCI process configuration.
type OCIProcess struct {
	Terminal        bool             `json:"terminal,omitempty"`
	User            OCIUser          `json:"user"`
	Args            []string         `json:"args"`
	Env             []string         `json:"env,omitempty"`
	Cwd             string           `json:"cwd"`
	Capabilities    *OCICapabilities `json:"capabilities,omitempty"`
	Rlimits         []OCIRlimit      `json:"rlimits,omitempty"`
	NoNewPrivileges bool             `json:"noNewPrivileges,omitempty"`
}

// OCIUser represents OCI user configuration.
type OCIUser struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

// OCICapabilities represents Linux capabilities.
type OCICapabilities struct {
	Bounding    []string `json:"bounding,omitempty"`
	Effective   []string `json:"effective,omitempty"`
	Inheritable []string `json:"inheritable,omitempty"`
	Permitted   []string `json:"permitted,omitempty"`
	Ambient     []string `json:"ambient,omitempty"`
}

// OCIRlimit represents a resource limit.
type OCIRlimit struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

// OCIRoot represents the root filesystem.
type OCIRoot struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly,omitempty"`
}

// OCIMount represents a mount point.
type OCIMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type,omitempty"`
	Source      string   `json:"source,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// OCILinux represents Linux-specific configuration.
type OCILinux struct {
	Namespaces    []OCINamespace    `json:"namespaces,omitempty"`
	Resources     *OCIResources     `json:"resources,omitempty"`
	CgroupsPath   string            `json:"cgroupsPath,omitempty"`
	Seccomp       *OCISeccomp       `json:"seccomp,omitempty"`
	MaskedPaths   []string          `json:"maskedPaths,omitempty"`
	ReadonlyPaths []string          `json:"readonlyPaths,omitempty"`
	Sysctl        map[string]string `json:"sysctl,omitempty"`
}

// OCINamespace represents a Linux namespace.
type OCINamespace struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

// OCIResources represents cgroup resources.
type OCIResources struct {
	Memory *OCIMemory `json:"memory,omitempty"`
	CPU    *OCICPU    `json:"cpu,omitempty"`
	Pids   *OCIPids   `json:"pids,omitempty"`
}

// OCIMemory represents memory resources.
type OCIMemory struct {
	Limit       *int64 `json:"limit,omitempty"`
	Reservation *int64 `json:"reservation,omitempty"`
	Swap        *int64 `json:"swap,omitempty"`
}

// OCICPU represents CPU resources.
type OCICPU struct {
	Shares *uint64 `json:"shares,omitempty"`
	Quota  *int64  `json:"quota,omitempty"`
	Period *uint64 `json:"period,omitempty"`
	Cpus   string  `json:"cpus,omitempty"`
	Mems   string  `json:"mems,omitempty"`
}

// OCIPids represents pids resources.
type OCIPids struct {
	Limit int64 `json:"limit"`
}

// OCISeccomp represents seccomp configuration.
type OCISeccomp struct {
	DefaultAction string              `json:"defaultAction"`
	Architectures []string            `json:"architectures,omitempty"`
	Syscalls      []OCISeccompSyscall `json:"syscalls,omitempty"`
}

// OCISeccompSyscall represents a seccomp syscall rule.
type OCISeccompSyscall struct {
	Names  []string        `json:"names"`
	Action string          `json:"action"`
	Args   []OCISeccompArg `json:"args,omitempty"`
}

// OCISeccompArg represents a seccomp argument.
type OCISeccompArg struct {
	Index    uint   `json:"index"`
	Value    uint64 `json:"value"`
	ValueTwo uint64 `json:"valueTwo,omitempty"`
	Op       string `json:"op"`
}

// generateOCISpec generates an OCI runtime specification.
func (r *DefaultRuntime) generateOCISpec(config *ContainerConfig, rootfs string) (*OCISpec, error) {
	args := config.Command
	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}
	if len(config.Args) > 0 {
		args = append(args, config.Args...)
	}

	cwd := config.WorkingDir
	if cwd == "" {
		cwd = "/"
	}

	uid, gid := parseUserGroup(config.User)

	spec := &OCISpec{
		OCIVersion: "1.0.2",
		Process: &OCIProcess{
			Terminal: config.Tty,
			User: OCIUser{
				UID: uint32(uid), // #nosec G115 -- bounded by construction; see the surrounding guard
				GID: uint32(gid), // #nosec G115 -- bounded by construction; see the surrounding guard
			},
			Args: args,
			Env:  config.Env,
			Cwd:  cwd,
			Capabilities: &OCICapabilities{
				Bounding:    defaultCapabilities(),
				Effective:   defaultCapabilities(),
				Inheritable: defaultCapabilities(),
				Permitted:   defaultCapabilities(),
			},
			NoNewPrivileges: true,
		},
		Root: &OCIRoot{
			Path:     rootfs,
			Readonly: false,
		},
		Hostname:    config.Name,
		Annotations: config.Annotations,
	}

	// Add default mounts
	spec.Mounts = []OCIMount{
		{
			Destination: "/proc",
			Type:        "proc",
			Source:      "proc",
		},
		{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
		},
		{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
		},
		{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "ro"},
		},
	}

	// Add user mounts
	for _, m := range config.Mounts {
		ociMount := OCIMount{
			Destination: m.Destination,
			Type:        string(m.Type),
			Source:      m.Source,
			Options:     m.Options,
		}
		spec.Mounts = append(spec.Mounts, ociMount)
	}

	// Linux configuration
	spec.Linux = &OCILinux{
		Namespaces: []OCINamespace{
			{Type: "pid"},
			{Type: "ipc"},
			{Type: "uts"},
			{Type: "mount"},
			{Type: "network"},
		},
		MaskedPaths: []string{
			"/proc/acpi",
			"/proc/kcore",
			"/proc/keys",
			"/proc/latency_stats",
			"/proc/timer_list",
			"/proc/timer_stats",
			"/proc/sched_debug",
			"/proc/scsi",
			"/sys/firmware",
		},
		ReadonlyPaths: []string{
			"/proc/asound",
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		},
	}

	// Add resources
	if config.Resources != nil {
		spec.Linux.Resources = &OCIResources{}

		if config.Resources.MemoryLimitBytes > 0 {
			limit := config.Resources.MemoryLimitBytes
			spec.Linux.Resources.Memory = &OCIMemory{
				Limit: &limit,
			}
			if config.Resources.MemoryReservationBytes > 0 {
				res := config.Resources.MemoryReservationBytes
				spec.Linux.Resources.Memory.Reservation = &res
			}
			if config.Resources.MemorySwapLimitBytes > 0 {
				swap := config.Resources.MemorySwapLimitBytes
				spec.Linux.Resources.Memory.Swap = &swap
			}
		}

		if config.Resources.CPUShares > 0 || config.Resources.CPUQuota > 0 {
			spec.Linux.Resources.CPU = &OCICPU{}
			if config.Resources.CPUShares > 0 {
				shares := uint64(config.Resources.CPUShares)
				spec.Linux.Resources.CPU.Shares = &shares
			}
			if config.Resources.CPUQuota > 0 {
				quota := config.Resources.CPUQuota
				spec.Linux.Resources.CPU.Quota = &quota
			}
			if config.Resources.CPUPeriod > 0 {
				period := uint64(config.Resources.CPUPeriod)
				spec.Linux.Resources.CPU.Period = &period
			}
			if config.Resources.CPUSetCPUs != "" {
				spec.Linux.Resources.CPU.Cpus = config.Resources.CPUSetCPUs
			}
			if config.Resources.CPUSetMems != "" {
				spec.Linux.Resources.CPU.Mems = config.Resources.CPUSetMems
			}
		}

		if config.Resources.PidsLimit > 0 {
			spec.Linux.Resources.Pids = &OCIPids{
				Limit: config.Resources.PidsLimit,
			}
		}
	}

	// Add Linux-specific config
	if config.Linux != nil {
		if config.Linux.ReadonlyRootfs {
			spec.Root.Readonly = true
		}
		if len(config.Linux.MaskedPaths) > 0 {
			spec.Linux.MaskedPaths = config.Linux.MaskedPaths
		}
		if len(config.Linux.ReadonlyPaths) > 0 {
			spec.Linux.ReadonlyPaths = config.Linux.ReadonlyPaths
		}
		if len(config.Linux.Sysctl) > 0 {
			spec.Linux.Sysctl = config.Linux.Sysctl
		}
		if config.Linux.CgroupsPath != "" {
			spec.Linux.CgroupsPath = config.Linux.CgroupsPath
		}

		// Apply capabilities if specified
		if config.Linux.Capabilities != nil {
			spec.Process.Capabilities = &OCICapabilities{
				Bounding:    config.Linux.Capabilities.Bounding,
				Effective:   config.Linux.Capabilities.Effective,
				Inheritable: config.Linux.Capabilities.Inheritable,
				Permitted:   config.Linux.Capabilities.Permitted,
				Ambient:     config.Linux.Capabilities.Ambient,
			}
		}

		// Apply seccomp if specified
		if config.Linux.Seccomp != nil {
			spec.Linux.Seccomp = convertSeccomp(config.Linux.Seccomp)
		}
	}

	return spec, nil
}

// defaultCapabilities returns the default capabilities for containers.
func defaultCapabilities() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FSETID",
		"CAP_FOWNER",
		"CAP_MKNOD",
		"CAP_NET_RAW",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_NET_BIND_SERVICE",
		"CAP_SYS_CHROOT",
		"CAP_KILL",
		"CAP_AUDIT_WRITE",
	}
}

// convertSeccomp converts LinuxSeccomp to OCISeccomp.
func convertSeccomp(seccomp *LinuxSeccomp) *OCISeccomp {
	if seccomp == nil {
		return nil
	}

	ociSeccomp := &OCISeccomp{
		DefaultAction: string(seccomp.DefaultAction),
		Architectures: seccomp.Architectures,
	}

	for _, sc := range seccomp.Syscalls {
		ociSc := OCISeccompSyscall{
			Names:  sc.Names,
			Action: string(sc.Action),
		}
		for _, arg := range sc.Args {
			ociSc.Args = append(ociSc.Args, OCISeccompArg{
				Index:    arg.Index,
				Value:    arg.Value,
				ValueTwo: arg.ValueTwo,
				Op:       string(arg.Op),
			})
		}
		ociSeccomp.Syscalls = append(ociSeccomp.Syscalls, ociSc)
	}

	return ociSeccomp
}

// MarshalOCISpec marshals an OCI spec to JSON.
func MarshalOCISpec(spec *OCISpec) ([]byte, error) {
	return json.MarshalIndent(spec, "", "  ")
}

// UnmarshalOCISpec unmarshals an OCI spec from JSON.
func UnmarshalOCISpec(data []byte) (*OCISpec, error) {
	var spec OCISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
