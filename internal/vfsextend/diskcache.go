package vfsextend

import (
	"github.com/linkxzhou/LessDB/internal/prom"
	"github.com/linkxzhou/LessDB/internal/utils"

	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

const DefaultPageSize = 1 << 12
const DefaultNoCacheSize = -1

var envPageSize = utils.GetEnviron("LESSDB_CACHE_PAGESIZE")
var envPeriodStr = utils.GetEnviron("LESSDB_CACHE_PERIOD")
var defaultPeriod = 1000 * time.Millisecond

type (
	DiskCacheHandler struct {
		PageSize int

		fileHandler FileHandler

		fileName           string
		fileSize           int64
		fileSizeUpdateTime time.Time

		f     *os.File
		pages sync.Map

		cacheEtag string
		cacheHit  int
		cacheMiss int

		rwMutex sync.RWMutex
	}

	FileHandler func(string) (*os.File, error)
)

func NewDiskCache(fileHandler FileHandler, fileSize int64) *DiskCacheHandler {
	h := &DiskCacheHandler{
		fileHandler: fileHandler,
		fileSize:    fileSize,
	}

	if h.PageSize < 1 {
		// Set page size
		h.PageSize = DefaultPageSize
		pageSize, err := strconv.ParseInt(envPageSize, 10, 64)
		if err == nil {
			h.PageSize = int(pageSize)
		}
	}

	if peroid, err := strconv.ParseInt(envPeriodStr, 10, 64); err == nil {
		defaultPeriod = time.Duration(peroid) * time.Millisecond
	}

	return h
}

func (h *DiskCacheHandler) newFileCache(etag string) (err error) {
	h.rwMutex.Lock()
	defer h.rwMutex.Unlock()

	if h.fileHandler != nil {
		oldf := h.f
		h.f, err = h.fileHandler(fmt.Sprintf("%v.%v", h.fileName, etag))
		if err == nil {
			h.cacheEtag = etag
			h.pages = *new(sync.Map)
		}
		if oldf != nil {
			oldf.Close()
		}
	}

	return err
}

func (h *DiskCacheHandler) Get(p []byte, off int64, fetcher VFSReadAt) (int, error) {
	// Cache filesize or size cache expire time > 1000ms
	if h.resize() {
		if _, sizeErr := h.Size(fetcher); sizeErr != nil {
			return 0, sizeErr
		}
	}

	t := prom.NewPromTrace(prom.RNameVFS, prom.TNameCacheGet)
	defer t.SysDurations()

	// Create new cache
	etag := fetcher.Etag()
	if h.EtagModify(etag) || h.f == nil {
		if err := h.newFileCache(etag); err != nil {
			return 0, err
		}
	}

	startPage, endPage := h.pagesForRange(off, len(p))

	firstMissingPage := int64(-1)
	lastMissingPage := int64(-1)
	for i := int64(startPage); i <= endPage; i++ {
		if v, ok := h.pages.Load(i); ok && v.(bool) {
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
		t.Code = prom.CodeCacheMiss
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

		// ReadAt has modify etag
		etag = fetcher.Etag()
		if h.EtagModify(etag) {
			h.cacheEtag = etag
			h.fileSize = DefaultNoCacheSize
			return 0, missCacheErr
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
		t.Code = prom.CodeCacheHit
		h.cacheHit++
	}

	return h.f.ReadAt(p, off)
}

func (h *DiskCacheHandler) Size(fetcher VFSReadAt) (int64, error) {
	t := prom.NewPromTrace(prom.RNameVFS, prom.TNameCacheSize)
	defer t.SysDurations()

	if h.resize() {
		t.Code = prom.CodeCacheMiss
		fileSize, sizeErr := fetcher.Size()
		if sizeErr != nil {
			return 0, sizeErr
		}
		h.fileSize = fileSize
		h.fileSizeUpdateTime = time.Now()
	} else {
		t.Code = prom.CodeCacheHit
	}

	return h.fileSize, nil
}

func (h *DiskCacheHandler) EtagModify(etag string) bool {
	return h.cacheEtag != etag
}

func (h *DiskCacheHandler) Set(name string) {
	h.fileName = name
}

func (h *DiskCacheHandler) pagesForRange(offset int64, size int) (startPage, endPage int64) {
	startPage = offset / int64(h.PageSize)
	endPage = (offset+int64(size))/int64(h.PageSize) + 1
	return startPage, endPage
}

func (h *DiskCacheHandler) resize() bool {
	return h.fileSize <= DefaultNoCacheSize ||
		time.Now().Sub(h.fileSizeUpdateTime) > defaultPeriod
}
