package fileops

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path"
	"sync"
	"syscall"
	"time"

	"campfire/internal/shared/archive"
	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
	"campfire/internal/shared/workprogress"
)

const extractionLimit = 1000

// extractionConflictWait bounds human decisions, not completed-result retention.
const extractionConflictWait = 15 * time.Minute

type extractionItem struct {
	entry                       model.ExtractionEntry
	target, temporary, revision string
	parent                      int
	conflict                    *model.ExtractionConflict
	status                      model.ExtractionOutcomeStatus
	message                     string
	mode                        fs.FileMode
}

// Session concurrency contract:
//   - Manager.mu owns operation membership and admission.
//   - mu protects lifecycle/results, items/indices, answers, temporary-directory
//     ownership and the timer. Answer workers are registered while mu is held.
//   - fsMu serializes scanning/publication and allocation accounting. Writers
//     take mu briefly for state changes, never while copying file contents.
//   - Lock order is Manager.mu -> mu or fsMu -> mu; never wait for fsMu while
//     holding mu, and never acquire Manager.mu while holding a session lock.
//   - Identity, source/destination/volume, context and cancellation are immutable
//     after admission.
//     sourceInfo/allocation are initialized by the scanner before entries are
//     exposed; sourceInfo then remains immutable and allocation uses fsMu.
//   - Terminal state stops new decisions; active writers retain the server lease
//     until they drain. Only then may pending temporary contents be removed.
//
// The runtime server-entry lock serializes request admission. The manager's
// shutdown waits for registered workers independently of browser/RPC lifetimes.
type extractionSession struct {
	fresh                                             bool
	progress                                          *workprogress.Snapshot
	mu                                                sync.Mutex
	fsMu                                              sync.Mutex
	id, owner, source, destination, volume, temporary string
	serverID                                          string
	state                                             model.ExtractionState
	message                                           string
	version                                           int64
	expires                                           time.Time
	timer                                             *time.Timer
	timerGeneration                                   uint64
	ctx                                               context.Context
	cancel                                            context.CancelFunc
	items                                             []*extractionItem
	indices                                           map[string]int
	answers                                           map[string]model.ExtractionRequest
	scanDone                                          bool
	active                                            int
	responseBytes                                     int
	allocation                                        *archiveAllocation
	sourceInfo                                        os.FileInfo
}

// Internal work phases are never published: snapshot maps them to pending.
const (
	extractionWriting   model.ExtractionOutcomeStatus = "writing"
	extractionResolving model.ExtractionOutcomeStatus = "resolving"
)

// Terminal results do not release the server until all writers have drained.
// The caller holds the session lock.
func (s *extractionSession) busy() bool {
	return s.active > 0 || !s.state.Terminal()
}

// Run only starts, observes or answers an operation. Archive decoding is owned
// by the operation, never by an RPC or the lifetime of an open browser request.
func (m *Manager) Run(ctx context.Context, d model.Deployment, volume string, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Destination}
	if q.Extraction == nil || len(q.Extraction.ID) < 32 || !utils.IsValidResourceID(q.Extraction.ID) || q.ExtractionOwner == "" {
		return result, Failure(model.FileInvalid, "A valid extraction operation and owner are required.")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return result, Failure(model.FileBusy, "File operations are shutting down.")
	}

	s := m.operations[q.Extraction.ID]
	if s != nil && (s.owner != q.ExtractionOwner || s.serverID != q.ServerID || s.source != q.Path || s.destination != q.Destination || s.fresh != q.Extraction.Fresh) {
		m.mu.Unlock()
		return result, Failure(model.FileNotFound, "Extraction operation not found.")
	}

	if s == nil {
		if q.Operation != model.FileOperationExtractStart {
			m.mu.Unlock()
			return result, Failure(model.FileNotFound, "Extraction operation is no longer available. Check completed files before starting again.")
		}

		for _, other := range m.operations {
			if other.serverID == q.ServerID {
				other.mu.Lock()
				active := other.busy()
				other.mu.Unlock()

				if active {
					m.mu.Unlock()
					return result, Failure(model.FileBusy, "An extraction is already active for this server.")
				}
			}
		}

		if len(m.operations) >= 128 {
			m.mu.Unlock()
			return result, Failure(model.FileBusy, "Too many extraction operations or recent results are retained. Try again later.")
		}

		if !ValidPath(q.Path, false) || !ValidPath(q.Destination, true) {
			m.mu.Unlock()
			return result, Failure(model.FileInvalid, "Choose safe archive and destination paths.")
		}

		var (
			operationContext context.Context
			cancel           context.CancelFunc
		)

		if q.Extraction.Fresh {
			operationContext, cancel = context.WithTimeout(m.ctx, 30*time.Minute)
		} else {
			operationContext, cancel = context.WithCancel(m.ctx)
		}

		s = &extractionSession{fresh: q.Extraction.Fresh, id: q.Extraction.ID, owner: q.ExtractionOwner, serverID: q.ServerID, source: q.Path, destination: q.Destination, volume: volume, state: model.ExtractionRunning, ctx: operationContext, cancel: cancel, indices: make(map[string]int), answers: make(map[string]model.ExtractionRequest), expires: time.Now().Add(extractionConflictWait), active: 1}
		if s.fresh {
			s.items = []*extractionItem{{entry: model.ExtractionEntry{Path: s.source}, target: s.destination, status: extractionWriting}}
		}

		m.operations[s.id] = s

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()

			if s.fresh {
				m.scanFreshExtraction(d, s)
			} else {
				m.scanExtraction(d, s)
			}
		}()
	}
	m.mu.Unlock()

	if q.Operation == model.FileOperationExtractCancel {
		s.cancel()
	}

	s.mu.Lock()

	wasTerminal := s.state.Terminal()
	if s.state == model.ExtractionWaiting && !time.Now().Before(s.expires) {
		s.stop(model.ExtractionExpired, "Extraction expired while waiting for a response. Completed entries were kept.")
	}

	if q.Operation == model.FileOperationExtractCancel && !s.state.Terminal() {
		s.stop(model.ExtractionCancelled, "Extraction cancelled. Completed entries were kept.")
	}

	if q.Operation == model.FileOperationExtractResolve && !s.state.Terminal() {
		if err := m.resolveExtraction(d, s, *q.Extraction); err != nil {
			s.mu.Unlock()
			return result, err
		}
	}

	result.Extraction = s.snapshot()
	rearm := !wasTerminal && s.state.Terminal()
	s.mu.Unlock()

	if rearm {
		m.armExtraction(s)
	}

	return result, nil
}

func (m *Manager) scanExtraction(d model.Deployment, s *extractionSession) {
	root, err := os.OpenRoot(s.volume)
	if err == nil {
		defer func() { _ = root.Close() }()

		err = ContainedPath(root, s.source, false)
		if err == nil {
			err = m.makeExtractionRoot(root, d, s.destination)
		}

		if err == nil {
			var input *os.File

			input, err = root.OpenFile(s.source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
			if err == nil {
				defer func() { _ = input.Close() }()

				info, statErr := input.Stat()
				if statErr != nil {
					m.finishExtractionScan(s, statErr)
					return
				}

				s.mu.Lock()
				s.sourceInfo = info
				s.mu.Unlock()

				s.allocation, err = newArchiveAllocation(s.ctx, s.volume, d.Configuration.DiskMB)
				if err != nil {
					m.finishExtractionScan(s, err)
					return
				}

				bytes := model.FileTransferLimit
				if d.Configuration.DiskMB > 0 {
					bytes = min(bytes, d.Configuration.DiskMB*1048576)
				}

				members := newArchiveMembers(archiveEntryLimit, bytes)
				ctx := workprogress.WithReporter(s.ctx, func(progress workprogress.Snapshot) {
					s.mu.Lock()
					defer s.mu.Unlock()

					if !s.state.Terminal() {
						s.progress = &progress
						s.version++
					}
				})
				err = ReadArchive(ctx, input, s.source, func(entry archive.Entry, reader io.Reader) error {
					name, size, mode := entry.Name, entry.Size, entry.Mode

					if err := s.ctx.Err(); err != nil {
						return err
					}

					name, err := members.accept(name, size, mode)
					if err != nil || name == "" {
						return err
					}

					s.fsMu.Lock()
					defer s.fsMu.Unlock()

					index, err := m.registerExtractionEntry(root, d, s, name, size, mode)
					if err != nil {
						return err
					}

					return m.processExtractionItem(root, d, s, index, reader)
				})
			}
		}
	}

	m.finishExtractionScan(s, err)
}

func (m *Manager) finishExtractionScan(s *extractionSession, err error) {
	s.mu.Lock()
	s.scanDone = true

	s.active--
	if err != nil && !s.state.Terminal() {
		// A decoder may fail after the last callback (CRC/trailer). Never describe
		// those already-published files as rolled back or the operation as successful.
		s.stop(model.ExtractionFailed, err.Error())
	}

	s.updateState()
	s.mu.Unlock()
	m.armExtraction(s)
}

func (m *Manager) makeExtractionRoot(root *os.Root, d model.Deployment, destination string) error {
	if err := ContainedPath(root, destination, true); err != nil {
		return err
	}

	info, _, err := DestinationSnapshot(root, destination)
	if err != nil {
		return err
	}

	if info != nil {
		if info.Kind == model.FileKindDirectory {
			return nil
		}

		return Failure(model.FileConflict, "The extraction destination is not a directory.")
	}

	if err := root.Mkdir(destination, 0700); err != nil {
		return err
	}

	return m.ownExtractionDirectory(root, d, destination)
}

func (m *Manager) ownExtractionDirectory(root *os.Root, d model.Deployment, name string) error {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return m.FileOwner(file, d.Ember.User)
}

// Implicit parent directories participate in conflicts just like explicit ones.
// An existing directory merges normally; only a blocking file needs a decision.
func (m *Manager) registerExtractionEntry(root *os.Root, d model.Deployment, s *extractionSession, name string, size int64, mode fs.FileMode) (int, error) {
	if parent := path.Dir(name); parent != "." {
		s.mu.Lock()
		_, exists := s.indices[parent]
		s.mu.Unlock()

		if !exists {
			index, err := m.registerExtractionEntry(root, d, s, parent, 0, fs.ModeDir|0700)
			if err != nil {
				return 0, err
			}

			if err := m.processExtractionItem(root, d, s, index, nil); err != nil {
				return 0, err
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Terminal() {
		return 0, context.Canceled
	}

	if index, exists := s.indices[name]; exists {
		if (s.items[index].entry.Kind == model.FileKindDirectory) != mode.IsDir() {
			return 0, Failure(model.FileInvalid, "Archive uses the same path as both a file and a directory.")
		}

		return index, nil
	}

	encoded, _ := json.Marshal(model.FileOutcome{Path: name, Destination: path.Join(s.destination, name), Status: model.ExtractionOutcomeOverwritten})
	if len(s.items) >= extractionLimit || s.responseBytes+len(encoded)+256 > model.FileLimit/2 {
		return 0, Failure(model.FileLimitExceeded, "Interactive extraction reached its 1,000-entry or result-size limit. Completed entries were kept.")
	}

	parent := -1
	if index, ok := s.indices[path.Dir(name)]; ok {
		parent = index
	}

	kind := model.FileKindFile
	if mode.IsDir() {
		kind = model.FileKindDirectory
	}

	index := len(s.items)
	s.indices[name] = index
	s.items = append(s.items, &extractionItem{entry: model.ExtractionEntry{Path: name, Kind: kind, Size: size}, target: path.Join(s.destination, name), parent: parent, status: model.ExtractionOutcomePending, mode: mode})
	s.responseBytes += len(encoded) + 256
	s.version++

	return index, nil
}

// Called with fsMu held, but not the state lock while copying: status and
// cancellation stay responsive even for a large current file.
func (m *Manager) processExtractionItem(root *os.Root, d model.Deployment, s *extractionSession, index int, reader io.Reader) error {
	s.mu.Lock()

	item := s.items[index]
	if s.state.Terminal() {
		s.mu.Unlock()
		return context.Canceled
	}

	if item.status != model.ExtractionOutcomePending || item.conflict != nil {
		s.mu.Unlock()
		return nil
	}

	blocked, skip := false, false

	if item.parent >= 0 {
		parent := s.items[item.parent]

		blocked, skip = parent.status == model.ExtractionOutcomePending || parent.status == extractionWriting || parent.status == extractionResolving, parent.status == model.ExtractionOutcomeSkipped
		if !blocked {
			item.target = path.Join(parent.target, path.Base(item.entry.Path))
		}
	}

	if skip {
		item.status = model.ExtractionOutcomeSkipped
		s.version++
		s.mu.Unlock()

		if reader != nil && item.entry.Kind == model.FileKindFile {
			_, err := io.CopyN(io.Discard, contextReader{s.ctx, reader}, item.entry.Size)
			return err
		}

		return nil
	}
	s.mu.Unlock()

	var existing *model.FileEntry

	revision := "absent"

	var err error
	if !blocked {
		if err = ContainedPath(root, item.target, true); err != nil {
			return err
		}

		existing, revision, err = DestinationSnapshot(root, item.target)
		if err != nil {
			return err
		}
	}

	merge := item.entry.Kind == model.FileKindDirectory && existing != nil && existing.Kind == model.FileKindDirectory

	conflict := !blocked && !merge && (existing != nil || revision != "absent" || item.target == s.source)
	if blocked || conflict {
		if err := s.buffer(item, reader); err != nil {
			return err
		}

		if conflict {
			protected, err := s.protectedDestination(root, item.target)
			if err != nil {
				return err
			}

			s.mu.Lock()
			s.setConflict(index, existing, revision, false, protected)
			s.mu.Unlock()
		}

		return nil
	}

	if merge {
		s.mu.Lock()
		item.status = model.ExtractionOutcomeMerged
		s.version++
		s.mu.Unlock()

		return nil
	}

	return m.writeExtractionItem(root, d, s, index, reader, "absent", model.ExtractionOutcomeExtracted)
}

func (m *Manager) writeExtractionItem(root *os.Root, d model.Deployment, s *extractionSession, index int, reader io.Reader, revision string, status model.ExtractionOutcomeStatus) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}

	stream := reader

	s.mu.Lock()
	item := s.items[index]
	item.status = extractionWriting
	s.version++
	s.mu.Unlock()

	var err error
	if item.entry.Kind == model.FileKindDirectory {
		err = root.Mkdir(item.target, 0700)
		if err == nil {
			err = m.ownExtractionDirectory(root, d, item.target)
		}
	} else {
		var input *os.File
		if reader == nil {
			input, err = os.Open(item.temporary)
			if err == nil {
				defer func() { _ = input.Close() }()

				reader = input
			}
		}

		if err == nil {
			err = s.allocation.refresh(s.ctx, s.volume)
		}

		var (
			existing *model.FileEntry
			current  string
		)
		if err == nil {
			// Reconcile capacity before this final observation, never between the
			// revision check and truncating a conflicting destination.
			existing, current, err = DestinationSnapshot(root, item.target)
		}

		if err == nil && current != revision {
			return s.changedDestination(root, index, stream, existing, current)
		}

		if err == nil {
			oldSize := int64(0)
			if existing != nil {
				oldSize = existing.Size
			}

			err = s.allocation.reserveBytes(oldSize, item.entry.Size)
		}

		if err == nil {
			flags := os.O_WRONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
			if revision == "absent" {
				flags |= os.O_CREATE | os.O_EXCL
			}

			var file *os.File

			file, err = root.OpenFile(item.target, flags, 0600)
			if err == nil {
				info, statErr := file.Stat()

				err = statErr
				if err == nil && s.sourceInfo != nil && os.SameFile(s.sourceInfo, info) {
					err = Failure(model.FileConflict, "The source archive cannot be replaced, including through a hardlink.")
				}

				if err == nil && revision != "absent" && StatRevision(info) != revision {
					err = Failure(model.FileConflict, "The destination changed before extraction could write it. Nothing was replaced.")
				}

				if err == nil {
					err = s.ctx.Err()
				}

				if err == nil && revision != "absent" {
					err = file.Truncate(0)
				}

				if err == nil {
					err = m.FileOwner(file, d.Ember.User)
				}

				if err == nil {
					_, err = io.CopyN(file, contextReader{s.ctx, reader}, item.entry.Size)
				}

				if err == nil {
					err = file.Chmod(ImportedMode(item.mode))
				}

				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
	}

	s.mu.Lock()
	if err != nil {
		item.status, item.message = model.ExtractionOutcomeFailed, err.Error()
		if s.ctx.Err() != nil {
			item.status = model.ExtractionOutcomeCancelled
			item.message = "Extraction stopped; a partial destination file may remain."
		}
	} else {
		item.status = status
		if item.temporary != "" {
			_ = os.Remove(item.temporary)
			item.temporary = ""
		}
	}

	s.version++
	if !s.state.Terminal() {
		s.expires = time.Now().Add(extractionConflictWait)
	}
	s.mu.Unlock()

	return err
}

// resolveExtraction is called with the state lock held. Accepted decisions are
// idempotent, so losing a reply cannot cause a second overwrite.
func (m *Manager) resolveExtraction(d model.Deployment, s *extractionSession, q model.ExtractionRequest) error {
	if previous, ok := s.answers[q.ConflictID]; ok {
		if previous.Action != q.Action || previous.Name != q.Name {
			return Failure(model.FileConflict, "That conflict already has an accepted decision.")
		}

		return nil
	}

	index := -1

	for i, item := range s.items {
		if item.conflict != nil && item.conflict.ID == q.ConflictID {
			index = i
			break
		}
	}

	if index < 0 {
		s.message = "The conflict changed. Review the current conflict again."
		s.version++

		return nil
	}

	item := s.items[index]

	root, err := os.OpenRoot(s.volume)
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	existing, revision, err := DestinationSnapshot(root, item.target)
	if err != nil {
		return err
	}

	protected, err := s.protectedDestination(root, item.target)
	if err != nil {
		return err
	}

	if q.Action != model.ExtractionActionSkip && revision != item.revision {
		s.setConflict(index, existing, revision, true, protected)
		s.message = "The destination changed. Review this conflict again."

		return nil
	}

	target, expected := item.target, revision

	switch q.Action {
	case model.ExtractionActionSkip:
	case model.ExtractionActionRename:
		if !ValidPath(q.Name, false) || path.Base(q.Name) != q.Name || q.Name == path.Base(item.target) {
			return Failure(model.FileInvalid, "Choose a different, safe filename.")
		}

		target = path.Join(path.Dir(item.target), q.Name)
		for i, other := range s.items {
			if i != index && other.target == target {
				s.message = "That name is reserved by another archive entry."
				s.version++

				return nil
			}
		}

		info, rev, err := DestinationSnapshot(root, target)
		if err != nil {
			return err
		}

		if info != nil || rev != "absent" {
			s.message = "That name is now occupied. Choose another name."
			s.version++

			return nil
		}

		expected = "absent"
	case model.ExtractionActionOverwrite:
		if existing == nil || existing.Kind != model.FileKindFile || item.entry.Kind != model.FileKindFile {
			return Failure(model.FileInvalid, "Only regular files can be overwritten.")
		}
	case model.ExtractionActionMerge:
		if existing == nil || existing.Kind != model.FileKindDirectory || item.entry.Kind != model.FileKindDirectory {
			return Failure(model.FileInvalid, "Only directories can be merged.")
		}
	case model.ExtractionActionExtract:
		if existing != nil || revision != "absent" {
			return Failure(model.FileConflict, "The destination is no longer absent.")
		}
	default:
		return Failure(model.FileInvalid, "Invalid extraction action.")
	}

	if q.Action != model.ExtractionActionSkip {
		protected, err := s.protectedDestination(root, target)
		if err != nil {
			return err
		}

		if protected {
			return Failure(model.FileConflict, "The source archive cannot be replaced, including through a hardlink.")
		}
	}

	s.answers[q.ConflictID] = q
	item.conflict = nil
	item.target = target

	item.status = extractionResolving
	if q.Action == model.ExtractionActionSkip {
		item.status = model.ExtractionOutcomeSkipped
	}

	s.message = ""
	s.version++
	s.expires = time.Now().Add(extractionConflictWait)
	s.active++

	s.state = model.ExtractionRunning
	s.stopTimer()

	m.wg.Add(1)
	go m.runExtractionAnswer(d, s, index, target, expected, q)

	return nil
}

// runExtractionAnswer executes an already-accepted answer. resolveExtraction
// records the idempotent decision and increments active before launching it.
// This worker owns filesystem serialization, descendant processing and completion;
// it starts without mu held and never holds mu while copying file contents.
func (m *Manager) runExtractionAnswer(d model.Deployment, s *extractionSession, index int, target, expected string, q model.ExtractionRequest) {
	defer m.wg.Done()

	s.fsMu.Lock()
	s.mu.Lock()
	item := s.items[index]
	s.mu.Unlock()

	root, err := os.OpenRoot(s.volume)
	if err == nil {
		defer func() { _ = root.Close() }()

		if q.Action != model.ExtractionActionSkip {
			err = s.ctx.Err()
			if err == nil {
				err = ContainedPath(root, target, true)
			}

			if err == nil {
				currentEntry, current, checkErr := DestinationSnapshot(root, target)

				err = checkErr
				if err == nil && current != expected {
					protected, protectErr := s.protectedDestination(root, target)

					err = protectErr
					if err == nil {
						s.mu.Lock()
						s.setConflict(index, currentEntry, current, true, protected)
						s.mu.Unlock()
					}
				} else if err == nil {
					status := model.ExtractionOutcomeExtracted
					if q.Action == model.ExtractionActionOverwrite {
						status = model.ExtractionOutcomeOverwritten
					}

					if q.Action == model.ExtractionActionRename {
						status = model.ExtractionOutcomeRenamed
					}

					if q.Action == model.ExtractionActionMerge {
						s.mu.Lock()
						item.status = model.ExtractionOutcomeMerged
						s.version++
						s.mu.Unlock()
					} else {
						err = m.writeExtractionItem(root, d, s, index, nil, expected, status)
					}
				}
			}
		}

		if err == nil {
			// Release every buffered descendant whose parent is now resolved. Other
			// queued conflicts stay pending; unrelated entries never wait for them.
			s.mu.Lock()
			count := len(s.items)
			s.mu.Unlock()

			for i := 0; i < count; i++ {
				if err = m.processExtractionItem(root, d, s, i, nil); err != nil {
					break
				}
			}
		}
	}
	s.fsMu.Unlock()
	s.mu.Lock()

	s.active--
	if err != nil && !s.state.Terminal() {
		s.stop(model.ExtractionFailed, err.Error())
	}

	s.updateState()
	s.mu.Unlock()
	m.armExtraction(s)
}

func (s *extractionSession) setConflict(index int, existing *model.FileEntry, revision string, changed, protected bool) {
	item := s.items[index]
	item.status = model.ExtractionOutcomePending
	item.revision = revision
	item.conflict = &model.ExtractionConflict{ID: rand.Text(), Path: item.entry.Path, Destination: item.target, Kind: item.entry.Kind, Size: item.entry.Size, Existing: existing, Changed: changed, Protected: protected}
	s.version++
}

func (s *extractionSession) updateState() {
	if s.state.Terminal() {
		if s.active == 0 {
			for _, item := range s.items {
				if item.status == model.ExtractionOutcomePending || item.status == extractionWriting || item.status == extractionResolving {
					item.status = model.ExtractionOutcomeCancelled
				}
			}

			s.cleanup()
		}

		return
	}

	pending := false

	for _, item := range s.items {
		if item.status == model.ExtractionOutcomePending || item.status == extractionWriting || item.status == extractionResolving {
			pending = true
			break
		}
	}

	state := model.ExtractionRunning
	if s.scanDone && s.active == 0 {
		state = model.ExtractionCompleted
		if pending {
			state = model.ExtractionWaiting
		}
	}

	if s.state != state {
		if state == model.ExtractionWaiting {
			s.expires = time.Now().Add(extractionConflictWait)
		}

		s.state = state
		s.version++
	}

	if state == model.ExtractionCompleted {
		s.cancel()
		s.cleanup()
	}
}
func (s *extractionSession) stop(state model.ExtractionState, message string) {
	s.stopTimer()
	s.state, s.message = state, message
	s.cancel()

	s.version++
	for _, item := range s.items {
		item.conflict = nil
		if item.status == model.ExtractionOutcomePending {
			item.status = model.ExtractionOutcomeCancelled
		}
	}

	s.updateState()
}
func (s *extractionSession) cleanup() {
	if s.temporary != "" {
		_ = os.RemoveAll(s.temporary)
		s.temporary = ""
	}
}
func (s *extractionSession) snapshot() *model.ExtractionStatus {
	out := &model.ExtractionStatus{ID: s.id, Version: s.version, State: s.state, ExpiresAt: s.expires, Outcomes: []model.FileOutcome{}, Message: s.message, WorkProgress: s.progress}
	for _, item := range s.items {
		status := item.status
		if status == extractionWriting || status == extractionResolving {
			status = model.ExtractionOutcomePending
		}

		out.Outcomes = append(out.Outcomes, model.FileOutcome{Path: item.entry.Path, Destination: item.target, Status: status, Message: item.message})
		if status != model.ExtractionOutcomePending {
			continue
		}

		out.RemainingEntries++

		if item.conflict == nil {
			continue
		}

		out.PendingConflicts++
		if out.Conflict == nil {
			value := *item.conflict
			if value.Existing != nil {
				entry := *value.Existing
				value.Existing = &entry
			}

			out.Conflict = &value
		}
	}

	return out
}
