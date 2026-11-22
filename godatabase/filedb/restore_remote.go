package filedb

import "os"

func (s *FileSyncer) PrepareRestore(projectID string) error {
	if s == nil {
		return nil
	}
	if err := os.Remove(s.localStatePath(projectID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.mu.Lock()
	st := s.stateOfLocked(projectID)
	st.restored = true
	st.gen++
	s.mu.Unlock()
	return nil
}
