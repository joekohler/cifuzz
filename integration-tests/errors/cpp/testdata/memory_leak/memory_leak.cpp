#include <string>
using namespace std;

void leak(string c) {
    if (c == "FUZZING") {
        int *node = (int *) malloc(500);
    }
}
