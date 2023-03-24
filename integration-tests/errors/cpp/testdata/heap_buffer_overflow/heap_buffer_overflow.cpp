#include <string>
#include <cstring>
using namespace std;

void overflow(string c) {
    if (c == "FUZZING") {
        char *s = (char *) malloc(1);
        strcpy(s, "too long");
        printf("%s\n", s);
    }
}
