#include <string>
using namespace std;

void memory(string c) {
    if (c == "FUZZING") {
        while(1) {
            int* ptr = new int(1);
        }
    }
}
