package build

import (
	"strings"
	"testing"
)

func statuses(b Build) string {
	var out []string
	for _, s := range Steps(b) {
		out = append(out, s.Status)
	}
	return strings.Join(out, ",")
}

func TestSteps(t *testing.T) {
	tests := []struct {
		name  string
		build Build
		want  string
	}{
		{"just submitted", Build{Status: "IN_PROGRESS", Phases: []Phase{phase("SUBMITTED", "", 0, -1)}},
			"running,pending,pending,pending"},
		{"between prepare phases", Build{Status: "IN_PROGRESS", Phases: []Phase{phase("SUBMITTED", "SUCCEEDED", 0, 1)}},
			"running,pending,pending,pending"},
		{"cloning", Build{Status: "IN_PROGRESS", Phases: append(prepared(), phase("PRE_BUILD", "", 10, -1))},
			"succeeded,running,pending,pending"},
		{"clone done, build not listed yet", Build{Status: "IN_PROGRESS",
			Phases: append(prepared(), phase("PRE_BUILD", "SUCCEEDED", 10, 13))},
			"succeeded,succeeded,pending,pending"},
		{"building", Build{Status: "IN_PROGRESS",
			Phases: append(prepared(), phase("PRE_BUILD", "SUCCEEDED", 10, 13), phase("BUILD", "", 13, -1))},
			"succeeded,succeeded,running,pending"},
		{"succeeded", succeeded(), "succeeded,succeeded,succeeded,succeeded"},
		{"failed in build", failedInBuild(), "succeeded,succeeded,failed,skipped"},
		{"clone failed", Build{Status: "FAILED",
			Phases: append(prepared(), phase("PRE_BUILD", "FAILED", 10, 12))},
			"succeeded,failed,skipped,skipped"},
		{"timed out while building", Build{Status: "TIMED_OUT",
			Phases: append(prepared(), phase("PRE_BUILD", "SUCCEEDED", 10, 13), phase("BUILD", "TIMED_OUT", 13, 1200))},
			"succeeded,succeeded,failed,skipped"},
		{"provisioning fault", Build{Status: "FAULT",
			Phases: []Phase{phase("SUBMITTED", "SUCCEEDED", 0, 1), phase("PROVISIONING", "FAULT", 1, 5)}},
			"failed,skipped,skipped,skipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statuses(tt.build); got != tt.want {
				t.Fatalf("steps = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestStepsKeepFailureMessageAndTimes(t *testing.T) {
	steps := Steps(failedInBuild())
	name, msg := failure(steps)
	if name != "Build image" || !strings.Contains(msg, "exit status 1") {
		t.Fatalf("failure = %q %q", name, msg)
	}
	if steps[0].StartedAt == nil || !steps[0].StartedAt.Equal(*at(0)) || !steps[0].FinishedAt.Equal(*at(10)) {
		t.Fatalf("prepare times = %v %v", steps[0].StartedAt, steps[0].FinishedAt)
	}
	if s := Steps(Build{Status: "IN_PROGRESS", Phases: []Phase{phase("SUBMITTED", "", 0, -1)}})[0]; s.FinishedAt != nil {
		t.Fatal("running step must not have an end time")
	}
}

func TestCloneOver(t *testing.T) {
	cloning := Build{Status: "IN_PROGRESS", Phases: append(prepared(), phase("PRE_BUILD", "", 10, -1))}
	cloned := Build{Status: "IN_PROGRESS", Phases: append(prepared(), phase("PRE_BUILD", "SUCCEEDED", 10, 13))}
	if cloneOver(Steps(cloning)) || !cloneOver(Steps(cloned)) || !cloneOver(Steps(failedInBuild())) {
		t.Fatal("cloneOver wrong")
	}
}
