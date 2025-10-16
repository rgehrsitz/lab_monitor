package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"labmonitor/internal/config"
)

// FirestoreStore persists configuration in a Firestore document.
// Document schema:
//
//	collection: config
//	document: current
//	fields: { payload: string(JSON), schema_version: int, updated_at: timestamp }
type FirestoreStore struct {
	client        *firestore.Client
	collection    string
	document      string
	schemaVersion int
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{
		client:        client,
		collection:    "config",
		document:      "current",
		schemaVersion: 1,
	}
}

func (s *FirestoreStore) docRef() *firestore.DocumentRef {
	return s.client.Collection(s.collection).Doc(s.document)
}

func (s *FirestoreStore) Get(ctx context.Context) (config.Config, string, error) {
	snap, err := s.docRef().Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return config.Config{}, "", errors.New("configuration not initialized")
		}
		return config.Config{}, "", fmt.Errorf("firestore get config: %w", err)
	}
	payload, ok := snap.Data()["payload"].(string)
	if !ok || payload == "" {
		return config.Config{}, "", errors.New("config payload missing")
	}
	var cfg config.Config
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return config.Config{}, "", fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, "", err
	}
	version := s.versionFromSnapshot(snap)
	return cfg, version, nil
}

func (s *FirestoreStore) Put(ctx context.Context, cfg config.Config, version string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	doc := s.docRef()
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Allow creation when no version provided.
				if version != "" {
					return ErrConflict{Message: "configuration created elsewhere"}
				}
				return tx.Set(doc, map[string]any{
					"payload":        string(payload),
					"schema_version": s.schemaVersion,
					"updated_at":     firestore.ServerTimestamp,
				})
			}
			return fmt.Errorf("fetch config for update: %w", err)
		}

		currentVersion := s.versionFromSnapshot(snap)
		if currentVersion != version {
			return ErrConflict{Message: "configuration updated by another process"}
		}

		return tx.Set(doc, map[string]any{
			"payload":        string(payload),
			"schema_version": s.schemaVersion,
			"updated_at":     firestore.ServerTimestamp,
		}, firestore.MergeAll)
	})
}

func (s *FirestoreStore) versionFromSnapshot(doc *firestore.DocumentSnapshot) string {
	if doc == nil {
		return ""
	}
	if !doc.UpdateTime.IsZero() {
		return doc.UpdateTime.Format(time.RFC3339Nano)
	}
	if !doc.CreateTime.IsZero() {
		return doc.CreateTime.Format(time.RFC3339Nano)
	}
	return doc.Ref.ID
}

// InitIfEmpty ensures the config document exists. Useful for bootstrapping.
func (s *FirestoreStore) InitIfEmpty(ctx context.Context, cfg config.Config) error {
	iter := s.client.Collection(s.collection).Documents(ctx)
	defer iter.Stop()
	_, err := iter.Next()
	if err == iterator.Done {
		return s.Put(ctx, cfg, "")
	}
	if err != nil {
		return fmt.Errorf("scan config collection: %w", err)
	}
	return nil
}
