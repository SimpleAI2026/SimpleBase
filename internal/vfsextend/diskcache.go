package vfsextend

import (
	"os"
	"strconv"
	"sync"

	"github.com/linkxzhou/LessDB/internal/utils"
)

const defaultPageSize = 1 << 10

var envPageSize = utils.GetEnviron("CACHE_PAGESIZE")

type DiskCacheHandler struct {
	PageSize int

	f        *os.File
	fileSize int64
	pages    sync.Map

	cacheHit  int
	cacheMiss int
}

func NewDiskCache(f *os.File, fileSize int64) *DiskCacheHandler {
	return &DiskCacheHandler{
		f:        f,
		fileSize: fileSize,
	}
}

func (h *DiskCacheHandler) Get(p []byte, off int64, fetcher VFSReadAt) (int, error) {
	if h.PageSize < 1 {
		// Set page size
		h.PageSize = defaultPageSize
		pageSize, err := strconv.ParseInt(envPageSize, 10, 64)
		if err == nil {
			h.PageSize = int(pageSize)
		}
	}

	// Cache filesize
	if h.fileSize < 0 {
		fileSize, sizeErr := fetcher.Size()
		if sizeErr != nil {
			return 0, sizeErr
		}
		h.fileSize = fileSize
	}
	startPage, endPage := h.pagesForRange(off, len(p))

	firstMissingPage := int64(-1)
	lastMissingPage := int64(-1)
	for i := int64(startPage); i <= endPage; i++ {
		v, ok := h.pages.Load(i)
		if ok && v.(bool) {
			continue
		}
		if firstMissingPage < 0 {
			firstMissingPage = i
		}
		if lastMissingPage < i {
			lastMissingPage = i
		}
	}

	lastPage := h.fileSize / int64(h.PageSize)
	if firstMissingPage >= 0 {
		h.cacheMiss++
		pageCount := (lastMissingPage + 1) - firstMissingPage
		size := pageCount * int64(h.PageSize)
		if lastMissingPage == lastPage {
			size = size - int64(h.PageSize) + (h.fileSize % int64(h.PageSize))
		}
		offset := firstMissingPage * int64(h.PageSize)
		if h.fileSize >= 0 && h.fileSize < (size+offset) {
			size = h.fileSize - offset
		}
		buffer := make([]byte, size)
		n, readAtErr := fetcher.ReadAt(buffer, offset)
		if readAtErr != nil {
			return 0, readAtErr
		}
		buffer = buffer[:n]
		if wn, writeErr := h.f.WriteAt(buffer, offset); writeErr != nil && wn != n {
			return 0, writeErr
		}
		fullPagesRead := n / h.PageSize
		for i := int64(0); i < int64(fullPagesRead); i++ {
			h.pages.Store(firstMissingPage+i, true)
		}
	} else {
		h.cacheHit++
	}

	return h.f.ReadAt(p, off)
}

func (h *DiskCacheHandler) Size(fetcher VFSReadAt) (int64, error) {
	if h.fileSize < 0 {
		fileSize, sizeErr := fetcher.Size()
		if sizeErr != nil {
			return 0, sizeErr
		}
		h.fileSize = fileSize
	}
	return h.fileSize, nil
}

func (h *DiskCacheHandler) pagesForRange(offset int64, size int) (startPage, endPage int64) {
	startPage = offset / int64(h.PageSize)
	endPage = (offset+int64(size))/int64(h.PageSize) + 1
	return startPage, endPage
}
