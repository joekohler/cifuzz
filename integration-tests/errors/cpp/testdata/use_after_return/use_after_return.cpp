#include <string>
using namespace std;

int *ptr;

void foo() {
    int local[100];
    ptr = &local[0];
}

void use_after_return(string c) {
    if (c == "FUZZING") {
        foo();
        *ptr = 42;
    }
}

