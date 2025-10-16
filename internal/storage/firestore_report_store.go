package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"labmonitor/internal/state"
)

// FirestoreReportStore persists report history to Firestore.
// Collection schema:
//
//	collection: reports
//	document id: <unix_nano>-<hash>
//	fields: { timestamp: time, payload: string(JSON), created_at: timestamp }
type FirestoreReportStore struct {
	client       *firestore.Client
	collection   string
	historyLimit int
}

func NewFirestoreReportStore(client *firestore.Client, historyLimit int) *FirestoreReportStore {
	if historyLimit <= 0 {
		historyLimit = 30
	}
	return &FirestoreReportStore{
		client:       client,
		collection:   "reports",
		historyLimit: historyLimit,
	}
}

func (s *FirestoreReportStore) colRef() *firestore.CollectionRef {
	return s.client.Collection(s.collection)
}

func (s *FirestoreReportStore) SaveReport(record state.ReportRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	docID := fmt.Sprintf("%d", record.Timestamp.UnixNano())
	if record.PromptDigest != "" {
		docID = fmt.Sprintf("%s-%s", docID, record.PromptDigest)
	}
	doc := s.colRef().Doc(docID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := doc.Set(ctx, map[string]any{
		"timestamp":  record.Timestamp,
		"payload":    string(payload),
		"created_at": firestore.ServerTimestamp,
	}); err != nil {
		return fmt.Errorf("firestore save report: %w", err)
	}
	go s.prune()
	return nil
}

func (s *FirestoreReportStore) LoadHistory() ([]state.ReportRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iter := s.colRef().OrderBy("timestamp", firestore.Asc).Documents(ctx)
	defer iter.Stop()
	var records []state.ReportRecord
	for {
		snap, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, fmt.Errorf("firestore list reports: %w", err)
		}
		rec, err := decodeReportSnapshot(snap)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if s.historyLimit > 0 && len(records) > s.historyLimit {
		records = records[len(records)-s.historyLimit:]
	}
	return records, nil
}

func (s *FirestoreReportStore) LatestReport() (*state.ReportRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	iter := s.colRef().OrderBy("timestamp", firestore.Desc).Limit(1).Documents(ctx)
	defer iter.Stop()
	snap, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, nil
		}
		return nil, fmt.Errorf("firestore latest report: %w", err)
	}
	rec, err := decodeReportSnapshot(snap)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *FirestoreReportStore) prune() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iter := s.colRef().OrderBy("timestamp", firestore.Desc).Offset(s.historyLimit).Documents(ctx)
	defer iter.Stop()
	for {
		snap, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return
		}
		_, _ = snap.Ref.Delete(ctx)
	}
}

func decodeReportSnapshot(snap *firestore.DocumentSnapshot) (state.ReportRecord, error) {
	payload, ok := snap.Data()["payload"].(string)
	if !ok || payload == "" {
		return state.ReportRecord{}, fmt.Errorf("report payload missing for %s", snap.Ref.ID)
	}
	var record state.ReportRecord
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return state.ReportRecord{}, fmt.Errorf("decode report: %w", err)
	}
	return record, nil
}
