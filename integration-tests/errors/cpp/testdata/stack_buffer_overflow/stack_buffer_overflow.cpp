#include <string>
#include <cstdio>
using namespace std;

void overflow(string c) {
    if (c == "FUZZING") {
        char s[12];
        snprintf(s, 100, "This string is too long for the buffer");
    }
}
