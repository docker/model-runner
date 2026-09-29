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
	si := 0
	if len(buffer) > int(w.capacity) {
		si = len(buffer) - int(w.capacity)
	}
	for _, b := range buffer[si:] {
		// When the buffer is full, the byte at w.write is the oldest one.
		// Drop it by moving the read position past it before overwriting.
		if w.size == w.capacity {
			if w.read+1 < w.capacity {
				w.read++
			} else {
				w.read = 0
			}
		} else {
			w.size++
		}
		w.buf[w.write] = b
		if w.write+1 < w.capacity {
			w.write++
		} else {
			w.write = 0
		}
		written++
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
			w.read++
		} else {
			w.read = 0
		}
		read++
	}
	w.size -= read
	if read == 0 {
		err = io.EOF
	}
	return int(read), err
}
