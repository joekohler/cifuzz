#include <string>
using namespace std;

void exhaustion(string c) {
    if (c == "FUZZING") {
        exhaustion(c);
    }
}
