package mwanachamataskmanager

func (m *taskManager) ForRun(runID string) (TaskManager, error) {
	scoped, err := m.store.ForRun(runID)
	if err != nil {
		return nil, err
	}
	run := *m
	run.store = scoped
	return &run, nil
}
