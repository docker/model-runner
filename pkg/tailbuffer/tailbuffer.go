package tailbuffer

import (
	"io"
	"sync"
)

type tailBuffer struct {
	lock     sync.Mutex
	buf      []byte
	capacity uint
	size     uint
	read     uint
	write    uint
}

func NewTailBuffer(size uint) io.ReadWriter {
	return &tailBuffer{
		buf:      make([]byte, size),
		capacity: size,
		size:     0,
		read:     0,
		write:    0,
	}
}

func (w *tailBuffer) Write(buffer []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	written := 0
	shouldPushRead := false
	si := 0
	if len(buffer) > int(w.capacity) {
		si = len(buffer) - int(w.capacity)
	}
	for _, b := range buffer[si:] {
		if shouldPushRead {
			if w.read+1 < w.capacity {
				w.read += 1
			} else {
				w.read = 0
			}
		}
		w.buf[w.write] = b
		if w.write+1 < w.capacity {
			w.write += 1
		} else {
			w.write = 0
		}
		w.size += 1
		if w.size > w.capacity {
			w.size = w.capacity
		}
		shouldPushRead = w.write == w.read
		written += 1
	}
	return si + written, nil
}

func (w *tailBuffer) Read(buffer []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	var err error
	read := uint(0)
	for read < w.size && int(read) < len(buffer) {
		buffer[read] = w.buf[w.read]
		if w.read+1 < w.capacity {
			w.read += 1
		} else {
			w.read = 0
		}
		read += 1
	}
	w.size -= read
	if read == 0 {
		err = io.EOF
	}
	return int(read), err
}
