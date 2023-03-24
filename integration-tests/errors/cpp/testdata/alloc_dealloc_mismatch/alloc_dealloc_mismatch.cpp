#include <string>
using namespace std;

void mismatch(string c) {
    if (c == "FUZZING") {
        char *val;
        free(val);
    }
}
