package websession

import (
	"context"
	"fmt"

	"code-kanban/model"
	"code-kanban/model/tables"
)

func (m *Manager) recoverPersistedEventProjections(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	var sessions []tables.WebSessionTable
	if err := db.WithContext(ctx).Find(&sessions).Error; err != nil {
		return err
	}
	for _, session := range sessions {
		tailSeq, err := m.store.latestEventSeq(session.ID)
		if err != nil {
			return fmt.Errorf("read web session %s projection tail: %w", session.ID, err)
		}
		if tailSeq <= session.LastEventSeq {
			continue
		}
		events, err := m.store.readEventsAfter(session.ID, session.LastEventSeq)
		if err != nil {
			return fmt.Errorf("read web session %s projection events: %w", session.ID, err)
		}
		for _, event := range events {
			retry := eventProjectionRetry{
				record: session,
				event:  event,
				stage:  eventProjectionDatabase,
			}
			if err := m.projectPersistedEventDatabase(ctx, session.ID, &retry); err != nil {
				return fmt.Errorf("recover web session %s event %d: %w", session.ID, event.Seq, err)
			}
			session.LastEventSeq = event.Seq
		}
		if session.LastEventSeq != tailSeq {
			return fmt.Errorf("web session %s projection tail was not fully recovered", session.ID)
		}
	}
	return nil
}
