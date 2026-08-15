package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.etcd.io/bbolt"
	"maunium.net/go/mautrix/id"
)

var (
	bucketRecords = []byte("records")
	bucketActive  = []byte("active")
	bucketSync    = []byte("matrix_sync")

	ErrAlreadyActive = errors.New("an odometer record is already active")
	ErrNoActive      = errors.New("no odometer record is active")
)

type Record struct {
	ID              uint64 `json:"id"`
	UserID          string `json:"user_id"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	Purpose         string `json:"purpose"`
	KilometersMilli int64  `json:"kilometers_milli"`
	CreatedAt       string `json:"created_at"`
}

type ActiveRecord struct {
	UserID             string `json:"user_id"`
	StartDate          string `json:"start_date"`
	StartOdometerMilli int64  `json:"start_odometer_milli"`
	Purpose            string `json:"purpose"`
	StartedAt          string `json:"started_at"`
}

type Store struct {
	db *bbolt.DB
}

func openStore(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{bucketRecords, bucketActive, bucketSync} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) StartOdometer(userID, date, purpose string, odometerMilli int64, now time.Time) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketActive)
		key := []byte(userID)
		if b.Get(key) != nil {
			return ErrAlreadyActive
		}
		rec := ActiveRecord{
			UserID:             userID,
			StartDate:          date,
			StartOdometerMilli: odometerMilli,
			Purpose:            purpose,
			StartedAt:          now.UTC().Format(time.RFC3339),
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
}

func (s *Store) EndOdometer(userID, date string, odometerMilli int64, now time.Time) (Record, error) {
	var completed Record
	err := s.db.Update(func(tx *bbolt.Tx) error {
		activeBucket := tx.Bucket(bucketActive)
		key := []byte(userID)
		data := activeBucket.Get(key)
		if data == nil {
			return ErrNoActive
		}

		var active ActiveRecord
		if err := json.Unmarshal(data, &active); err != nil {
			return fmt.Errorf("decode active record: %w", err)
		}
		if odometerMilli < active.StartOdometerMilli {
			return fmt.Errorf("ending odometer cannot be lower than starting odometer")
		}
		km := odometerMilli - active.StartOdometerMilli
		if km == 0 {
			return fmt.Errorf("trip distance is 0 km")
		}

		records := tx.Bucket(bucketRecords)
		seq, err := records.NextSequence()
		if err != nil {
			return err
		}
		completed = Record{
			ID:              seq,
			UserID:          userID,
			StartDate:       active.StartDate,
			EndDate:         date,
			Purpose:         active.Purpose,
			KilometersMilli: km,
			CreatedAt:       now.UTC().Format(time.RFC3339),
		}
		encoded, err := json.Marshal(completed)
		if err != nil {
			return err
		}
		if err := records.Put(uint64Key(seq), encoded); err != nil {
			return err
		}
		return activeBucket.Delete(key)
	})
	return completed, err
}

func (s *Store) AddStandalone(userID, date, purpose string, kmMilli int64, now time.Time) (Record, error) {
	var rec Record
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketRecords)
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		rec = Record{
			ID:              seq,
			UserID:          userID,
			StartDate:       date,
			EndDate:         date,
			Purpose:         purpose,
			KilometersMilli: kmMilli,
			CreatedAt:       now.UTC().Format(time.RFC3339),
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(uint64Key(seq), data)
	})
	return rec, err
}

func (s *Store) RecordsByUser() (map[string][]Record, error) {
	out := make(map[string][]Record)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketRecords)
		return b.ForEach(func(_, value []byte) error {
			var rec Record
			if err := json.Unmarshal(value, &rec); err != nil {
				return fmt.Errorf("decode mileage record: %w", err)
			}
			out[rec.UserID] = append(out[rec.UserID], rec)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool {
			if out[userID][i].StartDate == out[userID][j].StartDate {
				return out[userID][i].ID < out[userID][j].ID
			}
			return out[userID][i].StartDate < out[userID][j].StartDate
		})
	}
	return out, nil
}

func (s *Store) ResetMileage() (recordsDeleted, activeDeleted int, err error) {
	err = s.db.Update(func(tx *bbolt.Tx) error {
		count := func(bucket []byte) int {
			b := tx.Bucket(bucket)
			n := 0
			_ = b.ForEach(func(_, _ []byte) error { n++; return nil })
			return n
		}
		recordsDeleted = count(bucketRecords)
		activeDeleted = count(bucketActive)
		if err := tx.DeleteBucket(bucketRecords); err != nil {
			return err
		}
		if err := tx.DeleteBucket(bucketActive); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bucketRecords); err != nil {
			return err
		}
		_, err := tx.CreateBucket(bucketActive)
		return err
	})
	return
}

func uint64Key(v uint64) []byte {
	var key [8]byte
	binary.BigEndian.PutUint64(key[:], v)
	return key[:]
}

// The following four methods implement mautrix.SyncStore. Keeping the sync token
// on disk is important: it prevents command events from being replayed after a restart.
func (s *Store) SaveFilterID(_ context.Context, userID id.UserID, filterID string) error {
	return s.saveSyncValue(string(userID)+"/filter", filterID)
}
func (s *Store) LoadFilterID(_ context.Context, userID id.UserID) (string, error) {
	return s.loadSyncValue(string(userID) + "/filter")
}
func (s *Store) SaveNextBatch(_ context.Context, userID id.UserID, nextBatchToken string) error {
	return s.saveSyncValue(string(userID)+"/next_batch", nextBatchToken)
}
func (s *Store) LoadNextBatch(_ context.Context, userID id.UserID) (string, error) {
	return s.loadSyncValue(string(userID) + "/next_batch")
}

func (s *Store) saveSyncValue(key, value string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketSync).Put([]byte(key), []byte(value))
	})
}

func (s *Store) loadSyncValue(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bbolt.Tx) error {
		raw := tx.Bucket(bucketSync).Get([]byte(key))
		if raw != nil {
			value = string(raw)
		}
		return nil
	})
	return value, err
}
