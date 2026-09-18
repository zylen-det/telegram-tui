package kitty

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"reflect"
)

const chunkSize = 4096

func TransmitPNG(writer io.Writer, imageID uint32, rectangle image.Rectangle, source image.Image) (err error) {
	if isNilInterface(writer) {
		return errors.New("kitty: writer must not be nil")
	}
	if imageID == 0 {
		return errors.New("kitty: image ID must be nonzero")
	}
	if rectangle.Empty() || rectangle.Min.X < 0 || rectangle.Min.Y < 0 {
		return errors.New("kitty: placement rectangle must be non-empty and non-negative")
	}
	if isNilInterface(source) {
		return errors.New("kitty: source image must not be nil")
	}
	if source.Bounds().Empty() {
		return errors.New("kitty: source image must be non-empty")
	}

	var pngBuffer bytes.Buffer
	if encodeErr := png.Encode(&pngBuffer, source); encodeErr != nil {
		return fmt.Errorf("kitty: encode PNG: %w", encodeErr)
	}
	payload := base64.StdEncoding.EncodeToString(pngBuffer.Bytes())

	if err = writeString(writer, "\x1b7"); err != nil {
		return fmt.Errorf("kitty: save cursor: %w", err)
	}
	defer func() {
		restoreErr := writeString(writer, "\x1b8")
		if restoreErr == nil {
			return
		}
		restoreErr = fmt.Errorf("kitty: restore cursor: %w", restoreErr)
		if err == nil {
			err = restoreErr
			return
		}
		err = errors.Join(err, restoreErr)
	}()

	position := fmt.Sprintf("\x1b[%d;%dH", rectangle.Min.Y+1, rectangle.Min.X+1)
	if err = writeString(writer, position); err != nil {
		return fmt.Errorf("kitty: position cursor: %w", err)
	}

	for offset := 0; offset < len(payload); offset += chunkSize {
		end := min(offset+chunkSize, len(payload))
		more := 0
		if end < len(payload) {
			more = 1
		}

		controls := fmt.Sprintf("m=%d", more)
		if offset == 0 {
			controls = fmt.Sprintf(
				"a=T,f=100,i=%d,c=%d,r=%d,q=2,m=%d",
				imageID,
				rectangle.Dx(),
				rectangle.Dy(),
				more,
			)
		}
		sequence := "\x1b_G" + controls + ";" + payload[offset:end] + "\x1b\\"
		if err = writeString(writer, sequence); err != nil {
			return fmt.Errorf("kitty: transmit chunk: %w", err)
		}
	}

	return nil
}

func DeleteImage(writer io.Writer, imageID uint32) error {
	if isNilInterface(writer) {
		return errors.New("kitty: writer must not be nil")
	}
	if imageID == 0 {
		return errors.New("kitty: image ID must be nonzero")
	}
	sequence := fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", imageID)
	if err := writeString(writer, sequence); err != nil {
		return fmt.Errorf("kitty: delete image: %w", err)
	}
	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func writeString(writer io.Writer, value string) error {
	written, err := io.WriteString(writer, value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}
