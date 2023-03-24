#include <string>
using namespace std;

void double_free(string c) {
    if (c == "FUZZING") {
        int *ptr = (int *) malloc(sizeof(int));
        free(ptr);
        free(ptr);
    }
}