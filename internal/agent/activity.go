package agent

import "time"

// SessionActivity is a metadata snapshot. It neither starts a process nor
// reads a transcript, and never exposes another owner's sessions.
type SessionActivity struct {
	ID, ArtifactID, Title string
	UpdatedAt             time.Time
}

func (m *Manager) RunningSessions(ownerID int64) []SessionActivity {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SessionActivity{}
	for _, s := range m.sessions {
		if s.OwnerID != ownerID {
			continue
		}
		s.mu.Lock()
		if !s.closed && !s.stopping && (s.streaming || s.promptInFlight) {
			out = append(out, SessionActivity{ID: s.ID, ArtifactID: s.ArtifactID(), Title: s.title, UpdatedAt: s.lastActive})
		}
		s.mu.Unlock()
	}
	return out
}
