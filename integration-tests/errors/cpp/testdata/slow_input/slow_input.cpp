#ifdef _WIN32
#include <windows.h>
#else
#include <unistd.h>
#endif

using namespace std;

void slow(size_t size) {
    // Don't show the undefined behavior if the input is empty, so that
    // we can test that the fuzzer correctly emits the test input that
    // caused the undefined behavior.
    if (size == 0) {
        return;
    }

    #ifdef _WIN32
        Sleep(2000);
    #else
        sleep(10);
    #endif
}
