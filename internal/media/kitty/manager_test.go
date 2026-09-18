package kitty

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestManagerFirstShowSetsActiveImage(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if got := manager.ActiveID(); got != 0 {
		t.Fatalf("new manager ActiveID() = %d, want 0", got)
	}

	if err := manager.Show(image.Rect(2, 3, 6, 8), managerSource()); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got := manager.ActiveID(); got != 1 {
		t.Fatalf("ActiveID() = %d, want 1", got)
	}
	if !strings.Contains(writer.String(), "a=T,f=100,i=1,c=4,r=5,q=2,m=0") {
		t.Fatalf("Show() output does not transmit image ID 1: %q", truncate(writer.String(), 120))
	}
}

func TestManagerWithNilWriterReturnsShowErrorAndClearRemainsNoOp(t *testing.T) {
	manager := NewManager(nil)
	err := callWithoutPanic(t, func() error {
		return manager.Show(image.Rect(0, 0, 1, 1), managerSource())
	})
	if err == nil {
		t.Fatal("Show() error = nil, want nil writer error")
	}
	if got := manager.ActiveID(); got != 0 {
		t.Fatalf("ActiveID() = %d, want 0 after failed Show", got)
	}
	if manager.nextID != 2 {
		t.Fatalf("nextID = %d, want failed Show to consume ID 1", manager.nextID)
	}
	if err := callWithoutPanic(t, manager.Clear); err != nil {
		t.Fatalf("Clear() with no active image error = %v, want nil", err)
	}
}

func TestManagerShowReplacesActiveImage(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("first Show() error = %v", err)
	}
	writer.reset()

	if err := manager.Show(image.Rect(1, 1, 3, 4), managerSource()); err != nil {
		t.Fatalf("replacement Show() error = %v", err)
	}
	if got := manager.ActiveID(); got != 2 {
		t.Fatalf("ActiveID() = %d, want 2", got)
	}
	const deleteOld = "\x1b_Ga=d,d=i,i=1,q=2\x1b\\"
	output := writer.String()
	if !strings.HasPrefix(output, deleteOld) {
		t.Fatalf("replacement output prefix = %q, want delete of old image", truncate(output, len(deleteOld)+20))
	}
	if !strings.Contains(output, "a=T,f=100,i=2,c=2,r=3,q=2,m=0") {
		t.Fatalf("replacement output does not transmit image ID 2: %q", truncate(output, 140))
	}
}

func TestManagerClearDeletesActiveImageOnce(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	writer.reset()

	if err := manager.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if got := manager.ActiveID(); got != 0 {
		t.Fatalf("ActiveID() after Clear = %d, want 0", got)
	}
	const wantDelete = "\x1b_Ga=d,d=i,i=1,q=2\x1b\\"
	if got := writer.String(); got != wantDelete {
		t.Fatalf("Clear() output = %q, want %q", got, wantDelete)
	}

	writer.reset()
	if err := manager.Clear(); err != nil {
		t.Fatalf("second Clear() error = %v", err)
	}
	if got := writer.String(); got != "" {
		t.Fatalf("second Clear() output = %q, want no output", got)
	}
}

func TestManagerReplacementDeleteFailureKeepsOldImageAndID(t *testing.T) {
	deleteErr := errors.New("delete old failed")
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("first Show() error = %v", err)
	}
	writer.reset()
	writer.setFailure(func(p []byte) error {
		if string(p) == "\x1b_Ga=d,d=i,i=1,q=2\x1b\\" {
			return deleteErr
		}
		return nil
	})

	err := manager.Show(image.Rect(0, 0, 1, 1), managerSource())
	if !errors.Is(err, deleteErr) {
		t.Fatalf("replacement Show() error = %v, want delete error", err)
	}
	if got := manager.ActiveID(); got != 1 {
		t.Fatalf("ActiveID() = %d, want old image ID 1", got)
	}
	if manager.nextID != 2 {
		t.Fatalf("nextID = %d, want unconsumed ID 2", manager.nextID)
	}
	output := writer.String()
	if strings.Contains(output, "\x1b7") || strings.Contains(output, "a=T") {
		t.Fatalf("replacement transmitted a new image after delete failure: %q", truncate(output, 120))
	}
}

func TestManagerInitialTransmitFailureClearsAndCleansNewImage(t *testing.T) {
	transmitErr := errors.New("transmit failed")
	writer := &recordingWriter{}
	writer.setFailure(failTransmission(transmitErr))
	manager := NewManager(writer)

	err := manager.Show(image.Rect(0, 0, 1, 1), managerSource())
	if !errors.Is(err, transmitErr) {
		t.Fatalf("Show() error = %v, want transmit error", err)
	}
	if got := manager.ActiveID(); got != 0 {
		t.Fatalf("ActiveID() = %d, want 0", got)
	}
	if manager.nextID != 2 {
		t.Fatalf("nextID = %d, want consumed ID 1", manager.nextID)
	}
	assertCleanupAfterTransmit(t, writer.String(), 1)
}

func TestManagerReplacementTransmitFailureClearsAndCleansNewImage(t *testing.T) {
	transmitErr := errors.New("replacement transmit failed")
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("first Show() error = %v", err)
	}
	writer.reset()
	writer.setFailure(failTransmission(transmitErr))

	err := manager.Show(image.Rect(0, 0, 1, 1), managerSource())
	if !errors.Is(err, transmitErr) {
		t.Fatalf("replacement Show() error = %v, want transmit error", err)
	}
	if got := manager.ActiveID(); got != 0 {
		t.Fatalf("ActiveID() = %d, want 0 after failed replacement", got)
	}
	const deleteOld = "\x1b_Ga=d,d=i,i=1,q=2\x1b\\"
	if output := writer.String(); !strings.HasPrefix(output, deleteOld) {
		t.Fatalf("replacement output prefix = %q, want old image deletion", truncate(output, len(deleteOld)+20))
	}
	assertCleanupAfterTransmit(t, writer.String(), 2)
}

func TestManagerClearFailureKeepsActiveImage(t *testing.T) {
	deleteErr := errors.New("clear failed")
	writer := &recordingWriter{}
	manager := NewManager(writer)
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	writer.reset()
	writer.setFailure(func(p []byte) error {
		if bytes.HasPrefix(p, []byte(kittyAPCStart+"a=d,")) {
			return deleteErr
		}
		return nil
	})

	err := manager.Clear()
	if !errors.Is(err, deleteErr) {
		t.Fatalf("Clear() error = %v, want delete error", err)
	}
	if got := manager.ActiveID(); got != 1 {
		t.Fatalf("ActiveID() = %d, want retained image ID 1", got)
	}
}

func TestManagerImageIDWrapSkipsZero(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewManager(writer)
	manager.nextID = math.MaxUint32

	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("Show(max ID) error = %v", err)
	}
	if got := manager.ActiveID(); got != math.MaxUint32 {
		t.Fatalf("ActiveID() = %d, want %d", got, uint32(math.MaxUint32))
	}
	if manager.nextID != 1 {
		t.Fatalf("nextID after wrap = %d, want 1", manager.nextID)
	}
	if err := manager.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
		t.Fatalf("Show(after wrap) error = %v", err)
	}
	if got := manager.ActiveID(); got != 1 {
		t.Fatalf("ActiveID() after wrap = %d, want 1", got)
	}
}

func TestManagerConcurrentShowsUseNonzeroUniqueIDs(t *testing.T) {
	const showCount = 24
	writer := &recordingWriter{}
	manager := NewManager(writer)
	start := make(chan struct{})
	errorsOut := make(chan error, showCount*2)
	var waitGroup sync.WaitGroup

	for i := 0; i < showCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			if err := manager.Show(image.Rect(0, 0, 1, 1), managerSource()); err != nil {
				errorsOut <- err
			}
		}()
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for j := 0; j < 100; j++ {
				if id := manager.ActiveID(); id > showCount {
					errorsOut <- fmt.Errorf("ActiveID() = %d, want <= %d", id, showCount)
					return
				}
			}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsOut)
	for err := range errorsOut {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	matches := regexp.MustCompile(`\x1b_Ga=T,f=100,i=([0-9]+),`).FindAllStringSubmatch(writer.String(), -1)
	if len(matches) != showCount {
		t.Fatalf("transmit count = %d, want %d", len(matches), showCount)
	}
	ids := make(map[uint32]struct{}, showCount)
	for _, match := range matches {
		value, err := strconv.ParseUint(match[1], 10, 32)
		if err != nil {
			t.Fatalf("parse transmitted image ID %q: %v", match[1], err)
		}
		id := uint32(value)
		if id == 0 {
			t.Fatal("transmitted reserved image ID 0")
		}
		if _, duplicate := ids[id]; duplicate {
			t.Fatalf("transmitted duplicate image ID %d", id)
		}
		ids[id] = struct{}{}
	}
	if got := manager.ActiveID(); got == 0 {
		t.Fatal("ActiveID() = 0 after successful concurrent shows")
	}
}

func failTransmission(transmitErr error) func([]byte) error {
	return func(p []byte) error {
		if bytes.HasPrefix(p, []byte(kittyAPCStart+"a=T,")) {
			return transmitErr
		}
		return nil
	}
}

func assertCleanupAfterTransmit(t *testing.T, output string, imageID uint32) {
	t.Helper()
	transmit := strings.Index(output, kittyAPCStart+"a=T,")
	cleanup := strings.LastIndex(output, fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", imageID))
	if transmit < 0 {
		t.Fatal("output does not contain attempted image transmission")
	}
	if cleanup < transmit {
		t.Fatalf("output does not attempt cleanup for image ID %d after failed transmit: %q", imageID, truncate(output, 160))
	}
}

func managerSource() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 2, 2))
}

func (w *recordingWriter) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = nil
}

func (w *recordingWriter) setFailure(fail func([]byte) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fail = fail
}
