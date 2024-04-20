package vfsextend

type nopCacheHandler struct {
}

func (h *nopCacheHandler) Get(p []byte, off int64, fetcher VFSReadAt) (int, error) {
	return fetcher.ReadAt(p, off)
}

func (h *nopCacheHandler) Size(fetcher VFSReadAt) (int64, error) {
	return fetcher.Size()
}

func (h *nopCacheHandler) Set(name string) {
}
