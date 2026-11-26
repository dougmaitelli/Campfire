package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type idRange struct{ inside, outside, count uint64 }
type idMap []idRange

func parseIDMap(data []byte) (idMap, error) {
	var result idMap

	for line := range strings.Lines(strings.TrimSpace(string(data))) {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, errors.New("invalid Docker namespace ID map")
		}

		var values [3]uint64

		for i, s := range fields {
			n, err := strconv.ParseUint(s, 10, 32)
			if err != nil {
				return nil, err
			}

			values[i] = n
		}

		r := idRange{values[0], values[1], values[2]}
		if r.count == 0 || r.outside == 0 || r.outside+r.count > math.MaxUint32 || r.inside+r.count > math.MaxUint32 {
			return nil, errors.New("invalid rootless Docker ID range")
		}

		result = append(result, r)
	}

	return result, nil
}

func (m idMap) host(id uint64) (int, error) {
	for _, r := range m {
		if id >= r.inside && id-r.inside < r.count {
			return int(r.outside + id - r.inside), nil
		}
	}

	return 0, fmt.Errorf("container ID %d is outside Docker's namespace mapping", id)
}

type dockerNamespace struct {
	uid, gid   int
	uids, gids idMap
}

func (n *dockerNamespace) gameOwner(user string) (int, int, error) {
	u, g, both := strings.Cut(user, ":")
	if !both {
		g = u
	}

	uid, uerr := strconv.ParseUint(u, 10, 32)

	gid, gerr := strconv.ParseUint(g, 10, 32)
	if uerr != nil || gerr != nil || uid == 0 || uid == math.MaxUint32 || gid == math.MaxUint32 {
		return 0, 0, errors.New("invalid numeric game user")
	}

	if n == nil {
		return int(uid), int(gid), nil
	}

	hostUID, err := n.uids.host(uid)
	if err != nil {
		return 0, 0, err
	}

	hostGID, err := n.gids.host(gid)

	return hostUID, hostGID, err
}

func (m *Manager) assignFileOwner(file *os.File, user string) error {
	uid, gid, err := m.namespace.gameOwner(user)
	if err != nil {
		return err
	}

	return file.Chown(uid, gid)
}

// AllowDockerTraversal grants only the daemon account search access to a
// Spark-owned parent of a bind mount. Its private files retain their own modes.
func (m *Manager) AllowDockerTraversal(ctx context.Context, directory string) error {
	if m.namespace == nil {
		return nil
	}

	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	// With POSIX ACLs the group mode bits represent the current mask.
	mask := (info.Mode().Perm()>>3)&7 | 1

	acl := fmt.Sprintf("u:%d:--x,m::%o", m.namespace.uid, uint32(mask))
	if err := exec.CommandContext(ctx, "setfacl", "-m", acl, "--", directory).Run(); err != nil {
		return fmt.Errorf("grant rootless Docker traversal on %s (requires setfacl and filesystem ACLs): %w", directory, err)
	}

	return nil
}

// ConfigureDocker reads the local daemon's actual user namespace before
// accepting work. Only kernel-provided mappings are used for host chowns.
func (m *Manager) ConfigureDocker(ctx context.Context) error {
	if err := m.inspectDockerNamespace(ctx); err != nil {
		return err
	}

	return m.AllowDockerTraversal(ctx, m.Directory)
}

// CheckDocker validates the runtime before enrollment. Probe ACL support on the
// actual filesystems without changing permissions on a running Spark.
func CheckDocker(ctx context.Context, dataDir string) (string, error) {
	socket, err := dockerSocket()
	if err != nil {
		return "", err
	}

	m := Manager{Docker: Docker{}}
	if err := m.inspectDockerNamespace(ctx); err != nil {
		return "", err
	}

	if m.namespace == nil {
		return "rootful unix://" + socket, nil
	}

	for _, parent := range []string{dataDir, filepath.Join(dataDir, "runtime"), filepath.Join(dataDir, "game-integrations")} {
		probe, err := os.MkdirTemp(parent, ".docker-acl-")
		if errors.Is(err, os.ErrNotExist) && parent != dataDir {
			continue
		}

		if err != nil {
			return "", err
		}

		if err := errors.Join(m.AllowDockerTraversal(ctx, probe), os.Remove(probe)); err != nil {
			return "", err
		}
	}

	return "rootless unix://" + socket, nil
}

func (m *Manager) inspectDockerNamespace(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	output, err := m.Docker.Run(ctx, nil, "info", "--format", "{{json .}}")
	if err != nil {
		return fmt.Errorf("inspect Docker daemon at configured local socket: %w", err)
	}

	var info struct {
		SecurityOptions                                []string
		CgroupVersion, CgroupDriver                    string
		MemoryLimit, SwapLimit, CPUCfsQuota, PidsLimit bool
	}
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return fmt.Errorf("read Docker daemon information: %w", err)
	}

	hasOption := func(name string) bool {
		for _, option := range info.SecurityOptions {
			key, _, _ := strings.Cut(option, ",")
			if key == "name="+name {
				return true
			}
		}

		return false
	}
	if !hasOption("rootless") {
		if hasOption("userns") {
			return errors.New("rootful Docker with userns-remap is unsupported")
		}

		return nil
	}

	if info.CgroupVersion != "2" || info.CgroupDriver != "systemd" || !info.MemoryLimit || !info.SwapLimit || !info.CPUCfsQuota || !info.PidsLimit {
		return errors.New("rootless Docker requires cgroup v2 with systemd and delegated CPU, memory, swap and PID limits")
	}

	socket, err := dockerSocket()
	if err != nil {
		return err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	local, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("expected a local Unix connection to Docker")
	}

	raw, err := local.SyscallConn()
	if err != nil {
		return err
	}

	var (
		peer    *unix.Ucred
		peerErr error
	)

	if err := raw.Control(func(fd uintptr) { peer, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil {
		return err
	}

	if peerErr != nil {
		return peerErr
	}

	proc, err := os.OpenRoot(fmt.Sprintf("/proc/%d", peer.Pid))
	if err != nil {
		return err
	}

	defer func() { _ = proc.Close() }()

	readMap := func(name string, owner uint32) (idMap, error) {
		data, err := proc.ReadFile(name)
		if err != nil {
			return nil, err
		}

		ids, err := parseIDMap(data)
		if err != nil {
			return nil, err
		}

		root, err := ids.host(0)
		if err != nil || root != int(owner) {
			return nil, fmt.Errorf("rootless Docker requires the daemon's direct Unix socket (%s)", name)
		}

		return ids, nil
	}

	n := &dockerNamespace{uid: int(peer.Uid), gid: int(peer.Gid)}
	if n.uids, err = readMap("uid_map", peer.Uid); err != nil {
		return err
	}

	if n.gids, err = readMap("gid_map", peer.Gid); err != nil {
		return err
	}

	m.namespace = n

	return nil
}
