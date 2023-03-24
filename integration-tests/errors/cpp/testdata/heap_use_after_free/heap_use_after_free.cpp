#include <string>
using namespace std;

void use_after_free(string c) {
    if (c == "FUZZING") {
        char *s = (char *) malloc(1);
        free(s);
        ::printf("%s", s);
    }
}
