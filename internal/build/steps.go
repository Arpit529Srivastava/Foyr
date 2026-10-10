package build

import (
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// stepPhases maps each developer-facing step to the CodeBuild phases it covers (ADR-027).
var stepPhases = []struct {
	name   string
	phases []string
}{
	{"Prepare", []string{"SUBMITTED", "QUEUED", "PROVISIONING", "DOWNLOAD_SOURCE", "INSTALL"}},
	{"Clone", []string{"PRE_BUILD"}},
	{"Build image", []string{"BUILD"}},
	{"Push image", []string{"POST_BUILD"}},
}

// failedPhase reports whether a CodeBuild phase status is a failure.
func failedPhase(status string) bool {
	switch status {
	case "FAILED", "FAULT", "TIMED_OUT", "STOPPED", "CLIENT_ERROR":
		return true
	}
	return false
}

// Steps turns a build's phases into the developer-facing steps.
func Steps(b Build) []store.Step {
	byType := map[string]Phase{}
	for _, p := range b.Phases {
		byType[p.Type] = p
	}
	done := b.Done()
	steps := make([]store.Step, 0, len(stepPhases))
	for i, sp := range stepPhases {
		st := store.Step{Name: sp.name, Position: i + 1, Status: store.StepPending, LogRef: b.LogURL}
		seen, finished, failed := 0, 0, false
		var msgs []string
		for _, t := range sp.phases {
			p, ok := byType[t]
			if !ok {
				continue
			}
			seen++
			if p.Start != nil && (st.StartedAt == nil || p.Start.Before(*st.StartedAt)) {
				st.StartedAt = p.Start
			}
			if p.End != nil {
				finished++
				if st.FinishedAt == nil || p.End.After(*st.FinishedAt) {
					st.FinishedAt = p.End
				}
			}
			if failedPhase(p.Status) {
				failed = true
				if p.Message != "" {
					msgs = append(msgs, p.Message)
				}
			}
		}
		st.Message = strings.Join(msgs, "; ")
		switch {
		case failed:
			st.Status = store.StepFailed
		case seen == 0 && done:
			st.Status = store.StepSkipped
		case seen == 0:
			st.Status = store.StepPending
		case finished < seen:
			st.Status = store.StepRunning
		case done || lastPhaseOfStepDone(byType, sp.phases) || nextStarted(byType, i):
			st.Status = store.StepSucceeded
		default: // between two phases of the same step
			st.Status = store.StepRunning
		}
		if st.Status == store.StepRunning {
			st.FinishedAt = nil
		}
		steps = append(steps, st)
	}
	return steps
}

// nextStarted reports whether any phase of a later step has appeared.
func nextStarted(byType map[string]Phase, i int) bool {
	for _, sp := range stepPhases[i+1:] {
		for _, t := range sp.phases {
			if _, ok := byType[t]; ok {
				return true
			}
		}
	}
	return false
}

// lastPhaseOfStepDone reports whether the step's final phase has ended.
func lastPhaseOfStepDone(byType map[string]Phase, phases []string) bool {
	p, ok := byType[phases[len(phases)-1]]
	return ok && p.End != nil
}

// failure returns the first failed step and its message.
func failure(steps []store.Step) (name, msg string) {
	for _, s := range steps {
		if s.Status == store.StepFailed {
			return s.Name, s.Message
		}
	}
	return "", ""
}

// cloneOver reports whether the clone step has finished one way or another.
func cloneOver(steps []store.Step) bool {
	for _, s := range steps {
		if s.Name == "Clone" {
			return s.Status != store.StepPending && s.Status != store.StepRunning
		}
	}
	return false
}
