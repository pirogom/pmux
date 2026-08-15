package server

import (
	"log"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// orphanStillActiveExitCode mirrors stillActiveExitCode in session.go (STILL_ACTIVE).
const orphanStillActiveExitCode = 259

// orphanGracePeriod is how long a process left running by closePaneInternal's
// forceKill=false path is allowed to keep running before pmux terminates it.
// Gives the user time to notice its output / let it finish work.
const orphanGracePeriod = 5 * time.Minute

const orphanSweepInterval = 30 * time.Second

type orphanedProcess struct {
	pid        int
	sessionID  string
	paneID     string
	detectedAt time.Time
}

// OrphanTracker tracks processes that pmux deliberately chose not to kill
// (see closePaneInternal's forceKill=false path) because the ConPTY pipe
// closed while the process was still STILL_ACTIVE. Left unchecked these
// would run forever in the background with no owning pane; OrphanTracker
// gives them a grace period, then cleans them up.
type OrphanTracker struct {
	mu      sync.Mutex
	entries map[int]*orphanedProcess
	started bool
}

func NewOrphanTracker() *OrphanTracker {
	return &OrphanTracker{entries: make(map[int]*orphanedProcess)}
}

// Track registers a process pmux is no longer able to talk to (its ConPTY
// pipe is gone) but chose not to terminate because it was still running.
func (t *OrphanTracker) Track(pid int, sessionID, paneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries[pid] = &orphanedProcess{
		pid:        pid,
		sessionID:  sessionID,
		paneID:     paneID,
		detectedAt: time.Now(),
	}
	log.Printf("[orphan] Tracking orphaned process PID %d (pane %s, session %s); will be cleaned up after %s if still running", pid, paneID, sessionID, orphanGracePeriod)

	if !t.started {
		t.started = true
		go t.sweepLoop()
	}
}

func (t *OrphanTracker) sweepLoop() {
	ticker := time.NewTicker(orphanSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		t.sweep(false)
	}
}

// sweep checks tracked PIDs, dropping ones that already exited on their own.
// When force is true (server shutdown), still-running PIDs are terminated
// immediately regardless of grace period; otherwise only ones past
// orphanGracePeriod are terminated.
func (t *OrphanTracker) sweep(force bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for pid, entry := range t.entries {
		alive, exitCode := processAlive(pid)
		if !alive {
			log.Printf("[orphan] PID %d (pane %s) exited on its own (code %d); no longer tracked", pid, entry.paneID, exitCode)
			delete(t.entries, pid)
			continue
		}

		if force || time.Since(entry.detectedAt) >= orphanGracePeriod {
			if err := killPID(pid); err != nil {
				log.Printf("[orphan] failed to terminate leftover PID %d (pane %s): %v", pid, entry.paneID, err)
			} else {
				log.Printf("[orphan] terminated leftover PID %d (pane %s)", pid, entry.paneID)
			}
			delete(t.entries, pid)
		}
	}
}

// Shutdown terminates all currently-tracked orphaned processes immediately,
// ignoring the grace period. Call before the pmux server process exits so
// orphans don't outlive it silently.
func (t *OrphanTracker) Shutdown() {
	t.sweep(true)
}

func processAlive(pid int) (bool, uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, 0
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false, 0
	}
	return exitCode == orphanStillActiveExitCode, exitCode
}

func killPID(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
