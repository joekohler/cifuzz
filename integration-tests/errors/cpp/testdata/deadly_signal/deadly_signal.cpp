#include <string>
using namespace std;

void deadly(string c) {
    if (c == "FUZZING") {
        __builtin_trap(); // TODO: doesn't work on macos, will abort before throwing the fuzzer error
    }
}
