#include IMPL
#include <cstdio>
#include <cassert>
int main() {
    assert(joinParts({"a","bc","d"}, ',') == "a,bc,d");
    assert(joinParts({}, ',') == "");
    try { joinParts({"a",""}, ','); assert(false); } catch (const std::invalid_argument& e) { assert(std::string(e.what()) == "empty part"); }
    try { joinParts({std::string(1024,'x')}, ','); assert(false); } catch (const std::length_error&) {}
    assert(joinParts({std::string(1023,'x')}, ',').size() == 1023);
    assert(g_live == 0);
    std::puts("PASS");
}
