#include <string>
#include <cstring>
using namespace std;

struct{
    char name[1];
} test;

void overflow(string c) {
    if (c == "FUZZING") {
        char* char_array = new char[c.length() + 1];
        strcpy(char_array, c.c_str());
        strcpy(test.name, char_array);
    }
}
