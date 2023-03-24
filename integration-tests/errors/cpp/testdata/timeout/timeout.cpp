#include <string>
using namespace std;

void timeout(string c) {
    if (c == "FUZZING") {
        while (1);
    }
}