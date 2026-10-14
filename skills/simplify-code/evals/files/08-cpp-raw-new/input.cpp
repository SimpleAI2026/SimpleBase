#include <string>
#include <stdexcept>
#include <cstring>
#include <vector>

int g_live = 0;  // instrumentation: live Buffer objects
struct Buffer {
    char* data; size_t n;
    Buffer(size_t n_) : n(n_) { data = new char[n]; memset(data, 0, n); g_live++; }
    ~Buffer() { delete[] data; g_live--; }
};

// Builds a joined string; throws on empty part. Public API.
std::string joinParts(const std::vector<std::string>& parts, char sep) {
    Buffer* buf = new Buffer(1024);
    size_t pos = 0;
    for (size_t i = 0; i < parts.size(); i++) {
        if (parts[i].empty()) {
            delete buf;
            throw std::invalid_argument("empty part");
        }
        if (i > 0) { buf->data[pos] = sep; pos++; }
        if (pos + parts[i].size() >= buf->n) { delete buf; throw std::length_error("too long"); }
        memcpy(buf->data + pos, parts[i].c_str(), parts[i].size());
        pos += parts[i].size();
    }
    std::string out(buf->data, pos);
    delete buf;
    return out;
}
