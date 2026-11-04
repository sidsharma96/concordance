package capturefiles

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCopyUsesOneVerifiedReadAndExpectedDescriptor(t *testing.T) {
	source := writeCapture(t, description())
	destination := t.TempDir()
	calls := 0
	info, err := Copy(testContext(t), source, testLimits(), func(name string, body []byte) error {
		calls++
		if calls == 1 {
			// The producer disappears after the first sink call. Remaining output must
			// still come from the already-verified buffers, not newly opened source files.
			if err := os.RemoveAll(source); err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(destination, name), body, 0600)
	})
	if err != nil || info.PayloadFiles != 2 || calls != 3 || len(info.Sources) != 1 || info.Sources[0] != "docs" {
		t.Fatal(info, calls, err)
	}
	if info.Original == info.Descriptor {
		t.Fatal("transport names were not rewritten")
	}
	result, err := ReadDirectoryExpected(testContext(t), destination, info.Descriptor, testLimits())
	if err != nil || !reflect.DeepEqual(result, expected()) {
		t.Fatal(result, err)
	}
	wrong := info.Descriptor
	wrong.SHA256[0] ^= 1
	if _, err := ReadDirectoryExpected(testContext(t), destination, wrong, testLimits()); !errors.Is(err, ErrMismatch) {
		t.Fatal(err)
	}
	if _, err := ReadDirectoryExpected(testContext(t), destination, DescriptorRef{}, testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	t.Log("producer removed during copy; copied payloads unchanged; original Git paths preserved")
}
func TestCopyFailureAndAdmissionReturnZero(t *testing.T) {
	source := writeCapture(t, description())
	fault := errors.New("sink failed")
	info, err := Copy(testContext(t), source, testLimits(), func(string, []byte) error { return fault })
	if !errors.Is(err, fault) || !reflect.DeepEqual(info, CopyInfo{}) {
		t.Fatal(info, err)
	}
	if _, err := Copy(testContext(t), source, testLimits(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	called := false
	if err := os.WriteFile(filepath.Join(source, "pages/shared.html"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err = Copy(testContext(t), source, testLimits(), func(string, []byte) error { called = true; return nil })
	if err == nil || called || !reflect.DeepEqual(info, CopyInfo{}) {
		t.Fatal(info, called, err)
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if _, err := Copy(ctx, source, testLimits(), func(string, []byte) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestExpectedDescriptorRejectsChangedBytesBeforePayloads(t *testing.T) {
	source := writeCapture(t, description())
	var descriptor []byte
	info, err := Copy(testContext(t), source, testLimits(), func(name string, body []byte) error {
		if name == DescriptorName {
			descriptor = bytes.Clone(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Missing payloads cannot turn a descriptor-pin error into successful decode.
	root := t.TempDir()
	writeFile(t, root, DescriptorName, append(descriptor, ' '))
	if _, err := ReadDirectoryExpected(testContext(t), root, info.Descriptor, testLimits()); !errors.Is(err, ErrMismatch) {
		t.Fatal(err)
	}
}

func TestCopyAdmitsRewrittenDescriptorBeforeWriting(t *testing.T) {
	producer := writeCapture(t, description())
	original, err := os.ReadFile(filepath.Join(producer, DescriptorName))
	if err != nil {
		t.Fatal(err)
	}
	l := testLimits()
	l.MaxDescriptorBytes = len(original)
	called := false
	// The short recipe path grows when flattened, so this exact original-byte cap
	// admits the source descriptor but rejects its rewritten transport descriptor.
	if _, err := Copy(testContext(t), producer, l, func(string, []byte) error { called = true; return nil }); !errors.Is(err, ErrLimit) || called {
		t.Fatal("rewritten descriptor not admitted before sink", called, err)
	}
}
func TestCopyKeepsReferenceOccurrenceBudgets(t *testing.T) {
	producer := writeCapture(t, description())
	destination := t.TempDir()
	l := testLimits()
	l.MaxFiles = 3
	l.MaxPayloadBytes = int64(len(recipeBody) + 2*len(htmlBody))
	info, err := Copy(testContext(t), producer, l, func(name string, body []byte) error {
		return os.WriteFile(filepath.Join(destination, name), body, 0600)
	})
	if err != nil || info.PayloadFiles != 2 {
		t.Fatal(info, err)
	}
	if _, err := ReadDirectoryExpected(testContext(t), destination, info.Descriptor, l); err != nil {
		t.Fatal(err)
	}
	l.MaxPayloadBytes--
	if _, err := ReadDirectory(testContext(t), producer, l); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := ReadDirectoryExpected(testContext(t), destination, info.Descriptor, l); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
