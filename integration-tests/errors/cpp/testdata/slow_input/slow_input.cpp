#include <string>
#include <unistd.h>
using namespace std;

void slow(string c) {
    if (c == "FUZZING") {
        sleep(10);
    }
}
