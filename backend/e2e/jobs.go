//go:build e2e && !production

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

type jobFixture struct {
	model.JobResult
	archive string
	data    []byte
}

func (r *runtime) Job(ctx context.Context, _ string, q model.JobRequest) (model.JobResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jobs == nil {
		r.jobs = make(map[string]*jobFixture)
	}
	key := q.ID + "/" + q.ServerID
	work := r.jobs[key]
	if work == nil {
		if q.Operation == model.JobOperationStatus {
			return model.JobResult{ID: q.ID, ServerID: q.ServerID, State: model.JobStateAbsent}, nil
		}
		work = &jobFixture{JobResult: model.JobResult{ID: q.ID, ServerID: q.ServerID, Size: q.Size, Digest: q.Digest, State: model.JobStateReceiving}, archive: q.ArchiveName}
		r.jobs[key] = work
	}
	switch q.Operation {
	case model.JobOperationStatus:
	case model.JobOperationPrepare:
		if work.Size != q.Size || work.Digest != q.Digest {
			return work.JobResult, errors.New("archive identity changed")
		}
	case model.JobOperationWrite:
		if q.Offset < 0 || q.Offset > int64(len(work.data)) || q.Offset+int64(len(q.Data)) > work.Size {
			return work.JobResult, errors.New("invalid job offset")
		}
		if q.Offset == int64(len(work.data)) {
			work.data = append(work.data, q.Data...)
		} else if end := q.Offset + int64(len(q.Data)); end > int64(len(work.data)) || !bytes.Equal(work.data[q.Offset:end], q.Data) {
			return work.JobResult, errors.New("conflicting job replay")
		}
	case model.JobOperationImport:
		if work.State == model.JobStatePublished || work.State == model.JobStateActive {
			break
		}
		hash := sha256.Sum256(work.data)
		if int64(len(work.data)) != work.Size || hex.EncodeToString(hash[:]) != work.Digest {
			return work.JobResult, errors.New("invalid job archive")
		}
		manifest, err := backuparchive.Inspect(ctx, bytes.NewReader(work.data))
		if err != nil {
			return work.JobResult, err
		}
		r.archives[q.ServerID+"/"+work.archive] = bytes.Clone(work.data)
		r.backups[q.ServerID] = append(r.backups[q.ServerID], model.FileEntry{Name: work.archive, Kind: model.FileKindFile, Size: work.Size, ModifiedAt: fixed, Portable: true, BackupMode: manifest.Mode})
		work.State = model.JobStatePublished
	case model.JobOperationActivate:
		work.State = model.JobStateActive
	case model.JobOperationDiscard:
		delete(r.files, q.ServerID)
		delete(r.archives, q.ServerID+"/"+work.archive)
		work.data = nil
		work.State = model.JobStateDiscarded
	default:
		return work.JobResult, errors.New("unsupported fixture job operation")
	}
	work.Offset = int64(len(work.data))
	return work.JobResult, nil
}
