package runtime

import (
	"campfire/internal/shared/model"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in: requires a locally available Node image and a Docker daemon. Containers
// have no host mounts or network, and use bounded memory and process counts.
func TestLiveDockerCrashEvidenceAcrossAutomaticRestarts(t *testing.T) {
	image := os.Getenv("CAMPFIRE_TEST_CRASH_IMAGE")
	if image == "" {
		t.Skip("set CAMPFIRE_TEST_CRASH_IMAGE to a locally available Node image")
	}

	m, d := fileTestManager(t)
	d.ServerID = "crash-" + strconv.FormatInt(time.Now().UnixNano(), 16)
	m.Docker = Docker{}

	e, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	e.Installed = true
	if err := m.persist(d.ServerID, e); err != nil {
		t.Fatal(err)
	}

	if err := m.WatchCrashes(d); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, _ = m.Docker.Run(ctx, nil, "rm", "--force", container(d.ServerID))
	})

	run := func(script, memory string) string {
		t.Helper()

		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		_, _ = m.Docker.Run(ctx, nil, "rm", "--force", container(d.ServerID))

		output, err := m.Docker.Run(ctx, nil, "run", "--pull=never", "--detach", "--init", "--name", container(d.ServerID), "--label", "campfire.server="+d.ServerID, "--network", "none", "--memory", memory, "--memory-swap", memory, "--pids-limit", "64", "--restart", "unless-stopped", image, "node", "-e", script)
		if err != nil {
			t.Fatal(err)
		}

		return strings.TrimSpace(output)
	}
	await := func(code int, oom bool) {
		t.Helper()

		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			records, failure := m.crashSnapshot(d.ServerID)
			for _, record := range records {
				if record.ExitCode == code && (record.Reason == model.CrashReasonOutOfMemory) == oom && strings.Contains(record.Console, "crash-probe") {
					return
				}
			}

			if failure != "" {
				t.Log(failure)
			}

			time.Sleep(100 * time.Millisecond)
		}

		records, failure := m.crashSnapshot(d.ServerID)
		t.Fatalf("missing native crash evidence: %+v (%s)", records, failure)
	}

	run(`console.log("crash-probe"); process.exit(23)`, "64m")
	await(23, false)
	run(`console.log("crash-probe"); const allocations=[]; for (;;) allocations.push(Buffer.alloc(8*1024*1024,1))`, "64m")
	await(137, true)

	// A child can be OOM-killed while the game process survives. Docker leaves
	// OOMKilled=true even after that game is subsequently stopped successfully.
	id := run(`process.on("SIGTERM",()=>process.exit(0)); const {spawn}=require("node:child_process"); const child=spawn(process.execPath,["-e","const a=[]; for(;;) a.push(Buffer.alloc(8*1024*1024,1))"],{stdio:"inherit"}); child.on("exit",()=>console.log("child-oom-observed")); setInterval(()=>{},1000)`, "128m")

	deadline := time.Now().Add(30 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		output, err := m.Docker.Run(ctx, nil, "logs", id)

		cancel()

		if err == nil && strings.Contains(output, "child-oom-observed") {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("child OOM was not observed")
		}

		time.Sleep(100 * time.Millisecond)
	}

	check, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	text, err := m.Docker.Run(check, nil, "inspect", "--format", "{{.State.Running}} {{.State.OOMKilled}}", id)
	if err != nil || strings.TrimSpace(text) != "true true" {
		t.Fatal("expected surviving process with sticky Docker OOM flag", text, err)
	}

	if err := m.stop(check, d); err != nil {
		t.Fatal(err)
	}

	text, err = m.Docker.Run(check, nil, "inspect", "--format", "{{.State.ExitCode}}", id)
	if err != nil || strings.TrimSpace(text) != "0" {
		t.Fatal("expected clean managed stop", text, err)
	}

	observer := m.crashObserver()
	deadline = time.Now().Add(5 * time.Second)

	for {
		state, err := observer.state(d.ServerID)
		if err != nil {
			t.Fatal(err)
		}

		state.mu.Lock()
		observed := state.ContainerID == id && state.LastAction == "die"
		state.mu.Unlock()

		if observed {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("managed exit event was not processed")
		}

		time.Sleep(10 * time.Millisecond)
	}

	records, _ := m.crashSnapshot(d.ServerID)
	for _, record := range records {
		if record.ContainerID == id {
			t.Fatal("survived child OOM became a crash", record)
		}
	}
}
